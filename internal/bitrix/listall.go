package bitrix

import (
	"context"
	"encoding/json"
	"fmt"
)

// Page — сколько строк отдаёт портал за один вызов списочного метода.
const Page = 50

// ListAll читает все страницы списочного метода, но не больше limit строк
// (0 — без предела). Первая страница сообщает total, остальные идут пачками
// по 50 вызовов за запрос: 3 000 сделок — один вызов и одна пачка вместо 60
// вызовов подряд. Если метод total не сообщает, страницы идут по next.
//
// Возвращает строки и total (-1, если портал его не назвал).
func (c *Client) ListAll(ctx context.Context, method string, params map[string]any, limit int) ([]json.RawMessage, int, error) {
	first := withStart(params, 0)
	resp, err := c.Call(ctx, method, first)
	if err != nil {
		return nil, -1, err
	}
	items, key, err := Items(resp.Result)
	if err != nil {
		return nil, -1, err
	}
	total := -1
	if resp.Total != nil {
		total = *resp.Total
	}
	enough := func() bool { return limit > 0 && len(items) >= limit }

	if total > len(items) && !enough() {
		want := total
		if limit > 0 && limit < want {
			want = limit
		}
		cmds := map[string]string{}
		order := []string{}
		for start := Page; start < want; start += Page {
			name := fmt.Sprintf("p%07d", start)
			cmds[name] = method + "?" + Encode(withStart(params, start))
			order = append(order, name)
		}
		res, err := c.Batch(ctx, cmds, true)
		if err != nil {
			return nil, total, err
		}
		for _, name := range order {
			if e, ok := res.Errors[name]; ok {
				return nil, total, batchError(e)
			}
			page, _, err := itemsAt(res.Result[name], key)
			if err != nil {
				return nil, total, err
			}
			items = append(items, page...)
		}
	} else if total < 0 {
		next := resp.Next
		for next != nil && !enough() {
			r, err := c.Call(ctx, method, withStart(params, *next))
			if err != nil {
				return nil, total, err
			}
			page, _, err := itemsAt(r.Result, key)
			if err != nil {
				return nil, total, err
			}
			items = append(items, page...)
			next = r.Next
		}
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

func withStart(params map[string]any, start int) map[string]any {
	out := make(map[string]any, len(params)+1)
	for k, v := range params {
		out[k] = v
	}
	out["start"] = start
	return out
}

// Items — строки ответа списочного метода. Старые методы (crm.deal.list)
// отдают массив, новые — объект с массивом внутри: crm.item.list — items,
// tasks.task.list — tasks. key — имя такого поля ("" для массива).
func Items(result json.RawMessage) ([]json.RawMessage, string, error) {
	var arr []json.RawMessage
	if json.Unmarshal(result, &arr) == nil {
		return arr, "", nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(result, &obj); err != nil {
		return nil, "", fmt.Errorf("метод вернул не список")
	}
	for _, k := range []string{"items", "tasks", "result", "list"} {
		if v, ok := obj[k]; ok && json.Unmarshal(v, &arr) == nil {
			return arr, k, nil
		}
	}
	found := ""
	for k, v := range obj {
		var a []json.RawMessage
		if json.Unmarshal(v, &a) == nil {
			if found != "" {
				return nil, "", fmt.Errorf("в ответе несколько списков — это не списочный метод")
			}
			found, arr = k, a
		}
	}
	if found == "" {
		return nil, "", fmt.Errorf("метод вернул не список")
	}
	return arr, found, nil
}

func itemsAt(result json.RawMessage, key string) ([]json.RawMessage, string, error) {
	if key == "" {
		return Items(result)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(result, &obj); err != nil {
		return nil, "", fmt.Errorf("страница списка не разобралась")
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(obj[key], &arr); err != nil {
		return nil, "", fmt.Errorf("на странице списка нет поля %s", key)
	}
	return arr, key, nil
}

func batchError(raw json.RawMessage) error {
	var e struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	_ = json.Unmarshal(raw, &e)
	return errorFrom(400, e.Error, e.Description, nil)
}
