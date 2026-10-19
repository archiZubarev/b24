package bitrix

import "strings"

// IsRead — метод только читает данные портала.
//
// Делит вызовы на два инструмента MCP: bitrix_read можно разрешить без
// вопросов, а bitrix_write Claude Code каждый раз показывает человеку.
// Решение по последнему слову имени метода: get, list, fields и родственные —
// чтение. Всё незнакомое считается записью: лишний вопрос дешевле, чем
// незамеченная правка. Исключения ниже — методы, которые называются как
// чтение, но что-то меняют.
func IsRead(method string) bool {
	m := strings.ToLower(strings.TrimSpace(method))
	if readExact[m] {
		return true
	}
	if writeExact[m] {
		return false
	}
	last := m
	if i := strings.LastIndex(m, "."); i >= 0 {
		last = m[i+1:]
	}
	if readVerbs[last] {
		return true
	}
	for _, p := range []string{"get", "list"} {
		if strings.HasPrefix(last, p) && !strings.Contains(last, "externallink") && !strings.Contains(last, "publiclink") {
			return true
		}
	}
	return false
}

var readExact = map[string]bool{
	"profile": true, "scope": true, "methods": true, "method.get": true, "server.time": true,
	"app.info": true, "user.current": true, "user.access": true, "access.name": true,
}

// writeExact — похожи на чтение, но меняют портал.
var writeExact = map[string]bool{
	"im.dialog.read":                  true, // отмечает сообщения прочитанными
	"im.dialog.unread":                true,
	"im.recent.hide":                  true,
	"im.recent.pin":                   true,
	"im.recent.unread":                true,
	"disk.file.getexternallink":       true, // создаёт публичную ссылку
	"disk.folder.getexternallink":     true,
	"im.notify.read":                  true,
	"im.notify.read.list":             true,
	"crm.timeline.comment.list.mark":  true,
	"tasks.task.counters.get.refresh": true,
}

var readVerbs = map[string]bool{
	"get": true, "list": true, "fields": true, "search": true, "count": true, "current": true,
	"info": true, "time": true, "version": true, "userfields": true,
}
