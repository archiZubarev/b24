package bitrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client вызывает методы одного портала через один вебхук.
type Client struct {
	hook    Hook
	http    *http.Client
	pace    *pacer
	backoff time.Duration
}

// pause — промежуток между вызовами одного вебхука. Портал пускает около двух
// запросов в секунду и копит запас на короткие всплески; чаще — отвечает
// QUERY_LIMIT_EXCEEDED.
const pause = 350 * time.Millisecond

// pacer общий у всех клиентов одного вебхука в процессе: MCP-сервер живёт
// долго и заводит клиента на каждый вызов.
type pacer struct {
	mu   sync.Mutex
	next time.Time
}

var pacers sync.Map

// New — клиент по ссылке вебхука.
func New(hookURL string) (*Client, error) {
	h, err := ParseHook(hookURL)
	if err != nil {
		return nil, err
	}
	p, _ := pacers.LoadOrStore(h.URL, &pacer{})
	return &Client{
		hook:    h,
		http:    &http.Client{Timeout: 2 * time.Minute, CheckRedirect: noRedirect},
		pace:    p.(*pacer),
		backoff: 2 * time.Second,
	}, nil
}

// WithHTTPClient — тот же клиент поверх другого http.Client (тесты с заглушкой портала).
func (c *Client) WithHTTPClient(h *http.Client) *Client {
	cp := *c
	hc := *h
	hc.CheckRedirect = noRedirect
	cp.http = &hc
	cp.backoff = time.Millisecond
	return &cp
}

// Hook — разобранная ссылка (токен из неё не печатать).
func (c *Client) Hook() Hook { return c.hook }

// Портал не должен уводить запрос с токеном на чужой адрес.
func noRedirect(req *http.Request, via []*http.Request) error {
	req.Header.Del("Referer")
	if len(via) > 0 && req.URL.Host != via[0].URL.Host {
		return fmt.Errorf("портал перенаправил запрос на другой адрес: %s", req.URL.Host)
	}
	if len(via) >= 5 {
		return errors.New("слишком много перенаправлений")
	}
	return nil
}

// Response — ответ метода.
type Response struct {
	Result json.RawMessage `json:"result"`
	Total  *int            `json:"total,omitempty"`
	Next   *int            `json:"next,omitempty"`
	Time   struct {
		DateStart string `json:"date_start"`
	} `json:"time"`
}

// PortalTime — когда портал начал выполнять вызов, по его часам.
func (r *Response) PortalTime() time.Time {
	t, _ := time.Parse(time.RFC3339, r.Time.DateStart)
	return t
}

// Call вызывает метод. params — url.Values (обозначения в скобках, как в
// документации) или map[string]any (уходит JSON). nil — без параметров.
func (c *Client) Call(ctx context.Context, method string, params any) (*Response, error) {
	method = strings.TrimSpace(method)
	if method == "" {
		return nil, errors.New("не указан метод")
	}
	var (
		body        []byte
		contentType string
	)
	switch p := params.(type) {
	case nil:
		body, contentType = []byte("{}"), "application/json"
	case url.Values:
		body, contentType = []byte(p.Encode()), "application/x-www-form-urlencoded"
	case map[string]any:
		b, err := json.Marshal(p)
		if err != nil {
			return nil, fmt.Errorf("параметры не собрались в JSON: %w", err)
		}
		body, contentType = b, "application/json"
	default:
		return nil, fmt.Errorf("параметры неизвестного вида %T", params)
	}

	var last error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, time.Duration(attempt)*c.backoff); err != nil {
				return nil, err
			}
		}
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
		resp, err := c.post(ctx, method, body, contentType)
		c.answered()
		if err == nil {
			return resp, nil
		}
		var e *Error
		if errors.As(err, &e) && e.retry {
			last = err
			continue
		}
		return nil, err
	}
	return nil, last
}

func (c *Client) post(ctx context.Context, method string, body []byte, contentType string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.hook.URL+method+".json", bytes.NewReader(body))
	if err != nil {
		return nil, errors.New(Redact(err.Error()))
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{Code: "NETWORK", Text: "Битрикс не ответил: " + Redact(err.Error()), retry: true}
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 256<<20))
	if err != nil {
		return nil, &Error{Code: "NETWORK", Text: "ответ портала оборвался: " + Redact(err.Error()), retry: true}
	}
	var out struct {
		Response
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	decErr := json.Unmarshal(data, &out)
	if decErr == nil && out.Error == "" && res.StatusCode < 300 {
		return &out.Response, nil
	}
	return nil, errorFrom(res.StatusCode, out.Error, out.Description, decErr)
}

func (c *Client) wait(ctx context.Context) error {
	c.pace.mu.Lock()
	d := time.Until(c.pace.next)
	c.pace.mu.Unlock()
	return sleep(ctx, d)
}

func (c *Client) answered() {
	c.pace.mu.Lock()
	c.pace.next = time.Now().Add(pause)
	c.pace.mu.Unlock()
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
