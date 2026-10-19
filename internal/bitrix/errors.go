package bitrix

import (
	"fmt"
	"strings"
)

// Error — отказ портала. Text — по-русски, с подсказкой, что делать; Code —
// код Битрикса как есть, по нему удобно искать в документации.
type Error struct {
	Code   string
	Text   string
	Status int
	retry  bool
}

func (e *Error) Error() string { return e.Text }

// Kind — вид отказа для решения «повторить, чинить ключ или параметры».
func (e *Error) Kind() string {
	switch e.Code {
	case "NETWORK", "QUERY_LIMIT_EXCEEDED", "INTERNAL_SERVER_ERROR", "OPERATION_TIME_LIMIT":
		return "временный"
	case "expired_token", "invalid_token", "INVALID_CREDENTIALS", "NO_AUTH_FOUND", "KEY":
		return "ключ"
	case "insufficient_scope":
		return "права"
	case "WRONG_AUTH_TYPE", "APPLICATION_CONTEXT":
		return "недоступно вебхуку"
	}
	return "запрос"
}

func errorFrom(status int, code, desc string, decErr error) error {
	if code == "" && decErr != nil {
		if status >= 500 {
			return &Error{Code: "INTERNAL_SERVER_ERROR", Status: status, Text: fmt.Sprintf("портал ответил ошибкой %d — повторите позже", status), retry: true}
		}
		return &Error{Code: "BAD_RESPONSE", Status: status, Text: fmt.Sprintf("портал вернул не JSON (код %d) — проверьте адрес портала в ссылке вебхука", status)}
	}
	e := &Error{Code: code, Status: status}
	d := Redact(desc)
	switch code {
	case "QUERY_LIMIT_EXCEEDED":
		e.Text, e.retry = "слишком много запросов к порталу — подождите несколько секунд", true
	case "OPERATION_TIME_LIMIT":
		e.Text = "портал исчерпал лимит времени на этот метод — повторите через минуту или сузьте выборку"
	case "expired_token", "invalid_token", "INVALID_CREDENTIALS", "NO_AUTH_FOUND":
		e.Text = "портал не принял ключ: вебхук удалён, отключён, истёк срок или его пользователь уволен — создайте новый и добавьте: b24 add"
	case "WRONG_AUTH_TYPE":
		e.Text = "методу нужен контекст приложения — через вебхук он недоступен никогда, права тут не помогут (нужно локальное приложение)"
	case "insufficient_scope":
		e.Text = "у вебхука нет права на этот метод — откройте вебхук в Битриксе, отметьте нужное право (" + d + ") и сохраните"
	case "ERROR_METHOD_NOT_FOUND", "METHOD_NOT_FOUND":
		e.Text = "такого метода нет — проверьте имя по документации (bitrix-search в b24-dev-mcp)"
	case "ACCESS_DENIED":
		if strings.Contains(strings.ToLower(desc), "application context") {
			e.Code = "APPLICATION_CONTEXT"
			e.Text = "методу нужен контекст приложения — через вебхук он недоступен никогда, права тут не помогут (нужно локальное приложение)"
		} else {
			e.Text = "у пользователя вебхука нет доступа к этим данным в портале: " + d
		}
	case "NOT_FOUND", "ERROR_NOT_FOUND":
		e.Text = "не найдено: " + d
	case "INTERNAL_SERVER_ERROR", "ERROR_UNEXPECTED_ANSWER":
		e.Text, e.retry = "внутренняя ошибка портала — повторите позже: "+d, true
	default:
		if d == "" {
			d = "без пояснения"
		}
		e.Text = fmt.Sprintf("Битрикс: %s — %s", code, d)
	}
	if status == 401 && e.Code == "" {
		e.Code, e.Text = "INVALID_CREDENTIALS", "портал не принял ключ (401)"
	}
	return e
}
