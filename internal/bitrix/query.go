package bitrix

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
)

// Encode переводит параметры из JSON в строку запроса в обозначениях, которые
// понимает Битрикс: filter[STAGE_ID][0]=NEW&select[0]=ID. Нужна внутри batch:
// там каждая команда — строка «метод?параметры».
func Encode(params map[string]any) string {
	v := url.Values{}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		flatten(k, params[k], v)
	}
	return v.Encode()
}

func flatten(prefix string, val any, out url.Values) {
	switch t := val.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			flatten(prefix+"["+k+"]", t[k], out)
		}
	case []any:
		for i, x := range t {
			flatten(fmt.Sprintf("%s[%d]", prefix, i), x, out)
		}
	case []string:
		for i, x := range t {
			out.Add(fmt.Sprintf("%s[%d]", prefix, i), x)
		}
	case nil:
		out.Add(prefix, "")
	case bool:
		// Как http_build_query в PHP: true — 1, false — 0.
		if t {
			out.Add(prefix, "1")
		} else {
			out.Add(prefix, "0")
		}
	case float64:
		out.Add(prefix, strconv.FormatFloat(t, 'f', -1, 64))
	case int:
		out.Add(prefix, strconv.Itoa(t))
	case int64:
		out.Add(prefix, strconv.FormatInt(t, 10))
	case string:
		out.Add(prefix, t)
	default:
		out.Add(prefix, fmt.Sprint(t))
	}
}

// Unslash убирает экранирование косой черты, которым портал портит ссылки
// («https:\/\/…»): так ответ читается легче и короче. Экранированная обратная
// косая («\\/») не трогается.
func Unslash(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' && i+1 < len(b) {
			if b[i+1] == '/' {
				out = append(out, '/')
			} else {
				out = append(out, b[i], b[i+1])
			}
			i++
			continue
		}
		out = append(out, b[i])
	}
	return out
}
