package bitrix

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// BatchResult — ответы команд пачки по их именам.
type BatchResult struct {
	Result map[string]json.RawMessage `json:"result"`
	Errors map[string]json.RawMessage `json:"result_error,omitempty"`
	Total  map[string]int             `json:"result_total,omitempty"`
	Next   map[string]int             `json:"result_next,omitempty"`
}

// Batch выполняет команды пачками по 50 (больше портал за раз не берёт).
// Команда — строка «метод?параметры», например "crm.deal.get?id=12".
// halt — остановить пачку на первой ошибке.
func (c *Client) Batch(ctx context.Context, cmds map[string]string, halt bool) (*BatchResult, error) {
	names := make([]string, 0, len(cmds))
	for k := range cmds {
		names = append(names, k)
	}
	sort.Strings(names)
	out := &BatchResult{Result: map[string]json.RawMessage{}, Errors: map[string]json.RawMessage{}, Total: map[string]int{}, Next: map[string]int{}}
	for i := 0; i < len(names); i += 50 {
		part := map[string]any{}
		for _, n := range names[i:min(i+50, len(names))] {
			part[n] = cmds[n]
		}
		h := 0
		if halt {
			h = 1
		}
		resp, err := c.Call(ctx, "batch", map[string]any{"halt": h, "cmd": part})
		if err != nil {
			return nil, err
		}
		var r struct {
			Result json.RawMessage `json:"result"`
			Errors json.RawMessage `json:"result_error"`
			Total  json.RawMessage `json:"result_total"`
			Next   json.RawMessage `json:"result_next"`
		}
		if err := json.Unmarshal(resp.Result, &r); err != nil {
			return nil, fmt.Errorf("ответ batch не разобрался: %w", err)
		}
		mergeRaw(r.Result, out.Result)
		mergeRaw(r.Errors, out.Errors)
		mergeInt(r.Total, out.Total)
		mergeInt(r.Next, out.Next)
	}
	return out, nil
}

// Пустой набор портал отдаёт массивом [], непустой — объектом.
func mergeRaw(raw json.RawMessage, into map[string]json.RawMessage) {
	if len(raw) == 0 || raw[0] != '{' {
		return
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) == nil {
		for k, v := range m {
			into[k] = v
		}
	}
}

func mergeInt(raw json.RawMessage, into map[string]int) {
	if len(raw) == 0 || raw[0] != '{' {
		return
	}
	var m map[string]int
	if json.Unmarshal(raw, &m) == nil {
		for k, v := range m {
			into[k] = v
		}
	}
}

// BatchMethod — имя метода в команде пачки.
func BatchMethod(cmd string) string {
	m, _, _ := strings.Cut(strings.TrimSpace(cmd), "?")
	return strings.TrimSpace(m)
}
