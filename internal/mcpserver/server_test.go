package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/archiZubarev/b24/internal/bitrix"
	"github.com/archiZubarev/b24/internal/keys"
)

const token = "secretTOKEN42"

// session — сервер b24 на портале-заглушке и клиент MCP к нему.
func session(t *testing.T, calls *[]string) *mcp.ClientSession {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimSuffix(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], ".json")
		*calls = append(*calls, method)
		body, _ := io.ReadAll(r.Body)
		var p map[string]any
		_ = json.Unmarshal(body, &p)
		switch method {
		case "crm.deal.list":
			start, _ := p["start"].(float64)
			var rows []any
			for i := int(start); i < min(int(start)+50, 120); i++ {
				rows = append(rows, map[string]any{"ID": i, "URL": "https://x.ru/a/b"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": rows, "total": 120})
		case "batch":
			res := map[string]any{}
			for k, v := range p["cmd"].(map[string]any) {
				var rows []any
				start := 0
				if i := strings.Index(v.(string), "start="); i >= 0 {
					for _, ch := range v.(string)[i+6:] {
						if ch < '0' || ch > '9' {
							break
						}
						start = start*10 + int(ch-'0')
					}
				}
				for i := start; i < min(start+50, 120); i++ {
					rows = append(rows, map[string]any{"ID": i})
				}
				res[k] = rows
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"result": res, "result_error": []any{}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"result": map[string]any{"ok": method}})
		}
	}))
	t.Cleanup(srv.Close)
	hook := "https://" + strings.TrimPrefix(srv.URL, "https://") + "/rest/5/" + token + "/"
	st := &keys.Store{Default: "main", Keys: []keys.Key{{Name: "main", Portal: "test.bitrix24.ru", UserID: 5, UserName: "Тест", Scopes: []string{"crm"}}}}
	open := func(name string) (*bitrix.Client, *keys.Key, error) {
		k, err := st.Resolve(name)
		if err != nil {
			return nil, nil, err
		}
		c, err := bitrix.New(hook)
		if err != nil {
			return nil, nil, err
		}
		return c.WithHTTPClient(srv.Client()), k, nil
	}
	server := New("test", open, func() (*keys.Store, error) { return st, nil })
	ts, tc := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, ts, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "1"}, nil).Connect(ctx, tc, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	r, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var sb strings.Builder
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), r.IsError
}

func TestTools(t *testing.T) {
	var calls []string
	cs := session(t, &calls)

	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ro := map[string]bool{}
	for _, tl := range tools.Tools {
		ro[tl.Name] = tl.Annotations != nil && tl.Annotations.ReadOnlyHint
	}
	for name, want := range map[string]bool{"bitrix_keys": true, "bitrix_read": true, "bitrix_list_all": true, "bitrix_batch": true, "bitrix_write": false} {
		if got, ok := ro[name]; !ok || got != want {
			t.Errorf("%s: только чтение = %v (есть: %v), ждали %v", name, got, ok, want)
		}
	}

	out, isErr := callTool(t, cs, "bitrix_keys", nil)
	if isErr || !strings.Contains(out, "test.bitrix24.ru") || strings.Contains(out, token) {
		t.Errorf("bitrix_keys: %s", out)
	}

	out, isErr = callTool(t, cs, "bitrix_read", map[string]any{"method": "crm.deal.update", "params": map[string]any{"id": 1}})
	if !isErr || !strings.Contains(out, "bitrix_write") {
		t.Errorf("bitrix_read пропустил запись: %v %s", isErr, out)
	}
	for _, c := range calls {
		if c == "crm.deal.update" {
			t.Fatal("запись ушла на портал через bitrix_read")
		}
	}

	out, isErr = callTool(t, cs, "bitrix_write", map[string]any{"method": "crm.deal.update", "params": map[string]any{"id": 1}})
	if isErr || !strings.Contains(out, "crm.deal.update") {
		t.Errorf("bitrix_write: %v %s", isErr, out)
	}

	out, isErr = callTool(t, cs, "bitrix_list_all", map[string]any{"method": "crm.deal.list", "limit": 1000})
	var lst struct {
		Total, Returned int
	}
	_ = json.Unmarshal([]byte(out), &lst)
	if isErr || lst.Total != 120 || lst.Returned != 120 {
		t.Errorf("bitrix_list_all: %v %s", isErr, out[:min(len(out), 200)])
	}

	out, isErr = callTool(t, cs, "bitrix_read", map[string]any{"method": "crm.deal.list"})
	if isErr || strings.Contains(out, `\/`) || !strings.Contains(out, "https://x.ru/a/b") {
		t.Errorf("косые в ответе: %s", out[:min(len(out), 300)])
	}

	out, isErr = callTool(t, cs, "bitrix_batch", map[string]any{"commands": map[string]any{"a": "crm.deal.get?id=1", "b": "crm.deal.delete?id=2"}})
	if !isErr || !strings.Contains(out, "crm.deal.delete") {
		t.Errorf("bitrix_batch пропустил удаление: %v %s", isErr, out)
	}
}
