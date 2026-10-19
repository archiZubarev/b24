// Пакет bitrix — клиент REST Битрикс24 через входящий вебхук.
//
// Ссылка вебхука — это пароль: в ней токен, с которым любой сделает в портале
// всё, что разрешено вебхуку. Поэтому клиент никогда не печатает её целиком:
// в ошибках и списках токен заменяется точками (Redact).
package bitrix

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Hook — разобранная ссылка входящего вебхука
// https://<портал>/rest/<ID пользователя>/<токен>/.
type Hook struct {
	URL    string // нормализованная ссылка, с косой чертой в конце
	Host   string
	UserID int
	Token  string
}

var tokenRe = regexp.MustCompile(`^[A-Za-z0-9]{8,64}$`)

// ParseHook разбирает ссылку вебхука. Хвост с методом («…/profile.json») и
// пробелы по краям отбрасываются: такую ссылку часто копируют из примера.
func ParseHook(s string) (Hook, error) {
	s = strings.TrimSpace(s)
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return Hook{}, fmt.Errorf("это не ссылка входящего вебхука: нужна вида https://портал/rest/ID/токен/")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "rest" {
		return Hook{}, fmt.Errorf("в ссылке нет /rest/ID/токен/ — скопируйте её целиком из настроек вебхука")
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil || id <= 0 {
		return Hook{}, fmt.Errorf("в ссылке вебхука после /rest/ должен стоять номер пользователя")
	}
	if !tokenRe.MatchString(parts[2]) {
		return Hook{}, fmt.Errorf("в ссылке вебхука не похож на правду токен — скопируйте ссылку ещё раз")
	}
	host := strings.ToLower(u.Host)
	return Hook{
		URL:    fmt.Sprintf("https://%s/rest/%d/%s/", host, id, parts[2]),
		Host:   host,
		UserID: id,
		Token:  parts[2],
	}, nil
}

// Masked — ссылка без токена, её можно показывать.
func (h Hook) Masked() string {
	return fmt.Sprintf("https://%s/rest/%d/••••••/", h.Host, h.UserID)
}

var hookRe = regexp.MustCompile(`(/rest/\d+/)[A-Za-z0-9]{8,64}`)

// Redact прячет токены всех вебхуков в тексте: ошибки сети Go кладут в текст
// полный адрес запроса.
func Redact(s string) string {
	return hookRe.ReplaceAllString(s, "${1}••••••")
}
