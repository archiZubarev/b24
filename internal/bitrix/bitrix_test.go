package bitrix

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseHook(t *testing.T) {
	h, err := ParseHook("  https://Example.bitrix24.ru/rest/182/abcdEFGH1234/profile.json ")
	if err != nil {
		t.Fatal(err)
	}
	if h.URL != "https://example.bitrix24.ru/rest/182/abcdEFGH1234/" || h.UserID != 182 || h.Host != "example.bitrix24.ru" {
		t.Fatalf("%+v", h)
	}
	if strings.Contains(h.Masked(), "abcd") {
		t.Fatal("в Masked виден токен")
	}
	for _, bad := range []string{"", "http://x.ru/rest/1/abcdefgh/", "https://x.ru/rest/abc/abcdefgh/", "https://x.ru/api/1/abcdefgh/", "https://x.ru/rest/1/a-b/"} {
		if _, err := ParseHook(bad); err == nil {
			t.Errorf("принята плохая ссылка %q", bad)
		}
	}
}

func TestRedact(t *testing.T) {
	s := `Post "https://x.bitrix24.ru/rest/12/s3cretTOKEN99/crm.deal.list.json": dial tcp`
	if got := Redact(s); strings.Contains(got, "s3cret") || !strings.Contains(got, "/rest/12/••••••/") {
		t.Fatal(got)
	}
}

func TestIsRead(t *testing.T) {
	read := []string{"crm.deal.list", "crm.deal.get", "crm.item.list", "crm.deal.fields", "user.get", "profile", "scope",
		"disk.folder.getchildren", "im.dialog.messages.get", "tasks.task.list", "crm.stagehistory.list", "user.search", "user.current"}
	write := []string{"crm.deal.add", "crm.deal.update", "crm.deal.delete", "im.message.add", "im.dialog.read", "im.recent.hide",
		"disk.file.getExternalLink", "im.disk.folder.get", "bizproc.workflow.start", "tasks.task.complete", "crm.timeline.comment.add", "batch", "something.unknown"}
	for _, m := range read {
		if !IsRead(m) {
			t.Errorf("%s должен быть чтением", m)
		}
	}
	for _, m := range write {
		if IsRead(m) {
			t.Errorf("%s должен быть записью", m)
		}
	}
}

func TestEncode(t *testing.T) {
	got := Encode(map[string]any{
		"filter": map[string]any{"STAGE_ID": []any{"NEW", "WON"}, ">OPPORTUNITY": 1000.5},
		"select": []any{"ID", "TITLE"},
		"start":  50,
		"flag":   true,
	})
	q, _ := url.ParseQuery(got)
	want := map[string]string{"filter[STAGE_ID][0]": "NEW", "filter[STAGE_ID][1]": "WON", "filter[>OPPORTUNITY]": "1000.5",
		"select[0]": "ID", "select[1]": "TITLE", "start": "50", "flag": "1"}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s=%q, ждали %q (%s)", k, q.Get(k), v, got)
		}
	}
}

// fakePortal — портал-заглушка: handler получает метод и параметры.
func fakePortal(t *testing.T, handler func(method string, params map[string]any) (any, int)) (*Client, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		method := strings.TrimSuffix(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], ".json")
		body, _ := io.ReadAll(r.Body)
		params := map[string]any{}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			_ = json.Unmarshal(body, &params)
		} else {
			q, _ := url.ParseQuery(string(body))
			for k := range q {
				params[k] = q.Get(k)
			}
		}
		out, status := handler(method, params)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "https://")
	c, err := New("https://" + host + "/rest/7/testTOKEN123/")
	if err != nil {
		t.Fatal(err)
	}
	return c.WithHTTPClient(srv.Client()), &calls
}

func TestCallAndErrors(t *testing.T) {
	var limited int32
	c, _ := fakePortal(t, func(method string, p map[string]any) (any, int) {
		switch method {
		case "profile":
			return map[string]any{"result": map[string]any{"ID": "7"}, "time": map[string]any{"date_start": "2026-10-19T10:00:00+03:00"}}, 200
		case "crm.deal.get":
			if atomic.AddInt32(&limited, 1) == 1 {
				return map[string]any{"error": "QUERY_LIMIT_EXCEEDED", "error_description": "Too many requests"}, 503
			}
			return map[string]any{"result": map[string]any{"ID": p["id"]}}, 200
		case "crm.deal.update":
			return map[string]any{"error": "insufficient_scope", "error_description": "crm"}, 401
		case "user.get":
			return map[string]any{"error": "INVALID_CREDENTIALS", "error_description": "Invalid request credentials"}, 401
		case "event.bind":
			return map[string]any{"error": "WRONG_AUTH_TYPE", "error_description": "Current authorization type is denied for this method"}, 403
		case "bizproc.robot.add":
			return map[string]any{"error": "ACCESS_DENIED", "error_description": "Access denied! Application context required"}, 403
		}
		return map[string]any{"error": "ERROR_METHOD_NOT_FOUND", "error_description": "Method not found!"}, 404
	})
	ctx := context.Background()
	r, err := c.Call(ctx, "profile", nil)
	if err != nil || r.PortalTime().IsZero() {
		t.Fatalf("profile: %v %+v", err, r)
	}
	r, err = c.Call(ctx, "crm.deal.get", map[string]any{"id": 5})
	if err != nil || !strings.Contains(string(r.Result), "5") {
		t.Fatalf("повтор после лимита: %v", err)
	}
	for method, kind := range map[string]string{"crm.deal.update": "права", "user.get": "ключ", "no.such": "запрос",
		"event.bind": "недоступно вебхуку", "bizproc.robot.add": "недоступно вебхуку"} {
		_, err := c.Call(ctx, method, nil)
		e, ok := err.(*Error)
		if !ok || e.Kind() != kind {
			t.Errorf("%s: %v (%T), ждали вид %s", method, err, err, kind)
		}
		if strings.Contains(err.Error(), "testTOKEN") {
			t.Errorf("%s: в ошибке токен", method)
		}
	}
}

func TestBatchSplitsAndMerges(t *testing.T) {
	var batches int32
	c, _ := fakePortal(t, func(method string, p map[string]any) (any, int) {
		atomic.AddInt32(&batches, 1)
		cmd, _ := p["cmd"].(map[string]any)
		if len(cmd) > 50 {
			t.Errorf("в пачке %d команд", len(cmd))
		}
		res := map[string]any{}
		errs := map[string]any{}
		for k, v := range cmd {
			if strings.HasPrefix(v.(string), "bad.") {
				errs[k] = map[string]any{"error": "ERROR_METHOD_NOT_FOUND", "error_description": "no"}
				continue
			}
			res[k] = k
		}
		var e any = errs
		if len(errs) == 0 {
			e = []any{} // так портал отдаёт пустой набор
		}
		return map[string]any{"result": map[string]any{"result": res, "result_error": e, "result_total": []any{}, "result_next": []any{}}}, 200
	})
	cmds := map[string]string{"x": "bad.method"}
	for i := 0; i < 120; i++ {
		cmds[fmt.Sprintf("c%03d", i)] = "crm.deal.get?id=" + strconv.Itoa(i)
	}
	r, err := c.Batch(context.Background(), cmds, false)
	if err != nil {
		t.Fatal(err)
	}
	if batches != 3 || len(r.Result) != 120 || len(r.Errors) != 1 {
		t.Fatalf("пачек %d, ответов %d, ошибок %d", batches, len(r.Result), len(r.Errors))
	}
}

func TestListAll(t *testing.T) {
	rows := func(from, n int) []any {
		var out []any
		for i := from; i < from+n; i++ {
			out = append(out, map[string]any{"ID": i})
		}
		return out
	}
	page := func(start, total int) []any { return rows(start, min(Page, total-start)) }
	for _, c := range []struct {
		name  string
		total int
		wrap  string
		limit int
		want  int
	}{{"массив", 130, "", 0, 130}, {"items", 75, "items", 0, 75}, {"предел", 500, "", 120, 120}, {"одна страница", 12, "tasks", 0, 12}} {
		total := c.total
		cl, calls := fakePortal(t, func(method string, p map[string]any) (any, int) {
			wrap := func(v []any) any {
				if c.wrap == "" {
					return v
				}
				return map[string]any{c.wrap: v}
			}
			if method == "batch" {
				res := map[string]any{}
				for k, v := range p["cmd"].(map[string]any) {
					q, _ := url.ParseQuery(strings.SplitN(v.(string), "?", 2)[1])
					start, _ := strconv.Atoi(q.Get("start"))
					if q.Get("filter[STAGE_ID]") != "WON" {
						t.Errorf("в команде пачки потерялся фильтр: %s", v)
					}
					res[k] = wrap(page(start, total))
				}
				return map[string]any{"result": map[string]any{"result": res, "result_error": []any{}}}, 200
			}
			start := int(p["start"].(float64))
			return map[string]any{"result": wrap(page(start, total)), "total": total}, 200
		})
		items, gotTotal, err := cl.ListAll(context.Background(), "crm.deal.list", map[string]any{"filter": map[string]any{"STAGE_ID": "WON"}}, c.limit)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(items) != c.want || gotTotal != total {
			t.Errorf("%s: строк %d, total %d", c.name, len(items), gotTotal)
		}
		var last struct{ ID int }
		_ = json.Unmarshal(items[len(items)-1], &last)
		if last.ID != c.want-1 {
			t.Errorf("%s: последняя строка %d", c.name, last.ID)
		}
		if *calls > 2 {
			t.Errorf("%s: вызовов %d, ждали не больше двух", c.name, *calls)
		}
	}
}
