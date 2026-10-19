package bitrix

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ParseArgs собирает параметры метода из аргументов командной строки в
// обозначениях документации Битрикса: id=12, filter[STAGE_ID]=WON,
// filter[>DATE_MODIFY]=2026-10-01, select[]=ID select[]=TITLE.
func ParseArgs(args []string) (map[string]any, error) {
	out := map[string]any{}
	for _, a := range args {
		k, v, ok := strings.Cut(a, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("параметр «%s»: нужен вид ключ=значение", a)
		}
		path, err := splitKey(k)
		if err != nil {
			return nil, err
		}
		if err := put(out, path, v); err != nil {
			return nil, fmt.Errorf("параметр «%s»: %w", a, err)
		}
	}
	return out, nil
}

// ParseJSON читает параметры из JSON: строкой или из файла (@путь).
func ParseJSON(s string) (map[string]any, error) {
	if strings.HasPrefix(s, "@") {
		b, err := os.ReadFile(s[1:])
		if err != nil {
			return nil, err
		}
		s = string(b)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("параметры не JSON-объект: %w", err)
	}
	return m, nil
}

// Merge добавляет параметры b поверх a (вложенные объекты сливаются).
func Merge(a, b map[string]any) map[string]any {
	if a == nil {
		a = map[string]any{}
	}
	for k, v := range b {
		if am, ok := a[k].(map[string]any); ok {
			if bm, ok := v.(map[string]any); ok {
				a[k] = Merge(am, bm)
				continue
			}
		}
		a[k] = v
	}
	return a
}

// splitKey: "filter[STAGE_ID][]" → filter, STAGE_ID, "".
func splitKey(k string) ([]string, error) {
	i := strings.Index(k, "[")
	if i < 0 {
		return []string{k}, nil
	}
	path := []string{k[:i]}
	rest := k[i:]
	for rest != "" {
		if rest[0] != '[' {
			return nil, fmt.Errorf("ключ «%s»: после ] ждали [", k)
		}
		j := strings.Index(rest, "]")
		if j < 0 {
			return nil, fmt.Errorf("ключ «%s»: не закрыта скобка", k)
		}
		path = append(path, rest[1:j])
		rest = rest[j+1:]
	}
	return path, nil
}

func put(m map[string]any, path []string, v string) error {
	key := path[0]
	if len(path) == 1 {
		m[key] = v
		return nil
	}
	if path[1] == "" { // key[] — элемент списка
		if len(path) > 2 {
			return fmt.Errorf("[] бывает только последним")
		}
		list, _ := m[key].([]any)
		m[key] = append(list, v)
		return nil
	}
	child, ok := m[key].(map[string]any)
	if !ok {
		if _, exists := m[key]; exists {
			return fmt.Errorf("«%s» задан и значением, и объектом", key)
		}
		child = map[string]any{}
		m[key] = child
	}
	return put(child, path[1:], v)
}
