// Пакет mcpserver — MCP-сервер для Claude: вызовы Битрикс24 ключами из
// хранилища системы. Ссылка вебхука не покидает процесс: Claude видит только
// имена ключей и ответы портала.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/archiZubarev/b24/internal/bitrix"
	"github.com/archiZubarev/b24/internal/keys"
)

// Opener достаёт клиента по имени ключа ("" — ключ по умолчанию).
type Opener func(name string) (*bitrix.Client, *keys.Key, error)

// maxText — сколько символов ответа отдавать Claude за раз: больше — и
// ответ портала съест контекст разговора.
const maxText = 60000

type callIn struct {
	Key    string         `json:"key,omitempty" jsonschema:"имя ключа из bitrix_keys; пусто — ключ по умолчанию"`
	Method string         `json:"method" jsonschema:"метод REST, например crm.deal.list"`
	Params map[string]any `json:"params,omitempty" jsonschema:"параметры метода как в документации: {\"filter\":{\"STAGE_ID\":\"WON\"},\"select\":[\"ID\",\"TITLE\"]}"`
}

type listIn struct {
	Key    string         `json:"key,omitempty" jsonschema:"имя ключа; пусто — по умолчанию"`
	Method string         `json:"method" jsonschema:"списочный метод, например crm.deal.list или crm.item.list"`
	Params map[string]any `json:"params,omitempty" jsonschema:"filter, select, order и т.п.; start не нужен"`
	Limit  int            `json:"limit,omitempty" jsonschema:"сколько строк прочитать, по умолчанию 500, не больше 20000"`
}

type batchIn struct {
	Key      string            `json:"key,omitempty" jsonschema:"имя ключа; пусто — по умолчанию"`
	Commands map[string]string `json:"commands" jsonschema:"имя команды → \"метод?параметры\", например {\"d1\":\"crm.deal.get?id=12\"}; до 50 за раз, больше делится само"`
	Halt     bool              `json:"halt,omitempty" jsonschema:"остановиться на первой ошибке"`
}

// Run запускает сервер на stdin/stdout.
func Run(ctx context.Context, version string, open Opener, list func() (*keys.Store, error)) error {
	return New(version, open, list).Run(ctx, &mcp.StdioTransport{})
}

// New собирает сервер со всеми инструментами.
func New(version string, open Opener, list func() (*keys.Store, error)) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "b24", Title: "Битрикс24", Version: version}, &mcp.ServerOptions{
		Instructions: "Вызовы REST Битрикс24 ключами из хранилища системы. Сначала bitrix_keys — какие порталы доступны. " +
			"Параметры методов сверяйте с документацией (сервер b24-dev-mcp). Чтение — bitrix_read и bitrix_list_all, " +
			"изменения — только bitrix_write и только после того, как человек согласился с тем, что именно изменится.",
	})
	no, yes := false, true

	mcp.AddTool(s, &mcp.Tool{
		Name:        "bitrix_keys",
		Title:       "Ключи Битрикс24",
		Description: "Какие ключи (вебхуки) сохранены: имя, портал, пользователь, права, какой основной. Самих ссылок не показывает.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &no},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		st, err := list()
		if err != nil {
			return nil, nil, err
		}
		if len(st.Keys) == 0 {
			return text("Ключей нет. Человек добавляет ключ сам в терминале: b24 add <имя> (ссылку вебхука в чат не вставлять)."), nil, nil
		}
		type row struct {
			Name    string   `json:"name"`
			Portal  string   `json:"portal"`
			User    string   `json:"user"`
			Scopes  []string `json:"scopes"`
			Default bool     `json:"default,omitempty"`
		}
		var rows []row
		for _, k := range st.Keys {
			rows = append(rows, row{k.Name, k.Portal, fmt.Sprintf("%s (ID %d)", k.UserName, k.UserID), k.Scopes, strings.EqualFold(k.Name, st.Default)})
		}
		return jsonText(rows), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:  "bitrix_read",
		Title: "Прочитать из Битрикс24",
		Description: "Вызов метода, который только читает: *.get, *.list, *.fields, user.get, profile и т.п. " +
			"Методы, которые что-то меняют, этот инструмент не выполнит — для них bitrix_write.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &yes},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in callIn) (*mcp.CallToolResult, any, error) {
		if !bitrix.IsRead(in.Method) {
			return nil, nil, fmt.Errorf("метод %s меняет данные или неизвестен — вызовите его через bitrix_write, предварительно сказав человеку, что изменится", in.Method)
		}
		return call(ctx, open, in)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:  "bitrix_write",
		Title: "Изменить в Битрикс24",
		Description: "Вызов любого метода, в том числе меняющего данные: *.add, *.update, *.delete, im.message.add, batch и т.п. " +
			"Перед вызовом скажите человеку, что именно будет изменено, и дождитесь согласия.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &yes, OpenWorldHint: &yes},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in callIn) (*mcp.CallToolResult, any, error) {
		return call(ctx, open, in)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:  "bitrix_list_all",
		Title: "Прочитать список целиком",
		Description: "Все страницы списочного метода (по 50 строк) одним вызовом: первая страница и пачки по 50 страниц. " +
			"Ограничивайте select[] нужными полями — так ответ меньше и быстрее.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &yes},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listIn) (*mcp.CallToolResult, any, error) {
		if !bitrix.IsRead(in.Method) {
			return nil, nil, fmt.Errorf("%s — не списочный метод чтения", in.Method)
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 500
		}
		limit = min(limit, 20000)
		c, _, err := open(in.Key)
		if err != nil {
			return nil, nil, err
		}
		items, total, err := c.ListAll(ctx, in.Method, in.Params, limit)
		if err != nil {
			return nil, nil, explain(err)
		}
		return jsonText(map[string]any{"total": total, "returned": len(items), "items": items}), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:  "bitrix_batch",
		Title: "Пачка запросов на чтение",
		Description: "Несколько методов чтения одним запросом (batch). Для пачки с изменениями — bitrix_write с методом batch. " +
			"Внутри команды можно ссылаться на ответ другой: crm.deal.get?id=$result[d1][0][ID].",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &yes},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in batchIn) (*mcp.CallToolResult, any, error) {
		if len(in.Commands) == 0 {
			return nil, nil, errors.New("нет команд")
		}
		var writes []string
		for name, cmd := range in.Commands {
			if !bitrix.IsRead(bitrix.BatchMethod(cmd)) {
				writes = append(writes, name+": "+bitrix.BatchMethod(cmd))
			}
		}
		if len(writes) > 0 {
			sort.Strings(writes)
			return nil, nil, fmt.Errorf("в пачке есть методы, которые меняют данные (%s) — для них bitrix_write с методом batch", strings.Join(writes, ", "))
		}
		c, _, err := open(in.Key)
		if err != nil {
			return nil, nil, err
		}
		res, err := c.Batch(ctx, in.Commands, in.Halt)
		if err != nil {
			return nil, nil, explain(err)
		}
		return jsonText(res), nil, nil
	})

	return s
}

func call(ctx context.Context, open Opener, in callIn) (*mcp.CallToolResult, any, error) {
	c, _, err := open(in.Key)
	if err != nil {
		return nil, nil, err
	}
	var params any
	if len(in.Params) > 0 {
		params = in.Params
	}
	r, err := c.Call(ctx, in.Method, params)
	if err != nil {
		return nil, nil, explain(err)
	}
	out := map[string]any{"result": r.Result}
	if r.Total != nil {
		out["total"] = *r.Total
	}
	if r.Next != nil {
		out["next"] = *r.Next
	}
	return jsonText(out), nil, nil
}

// explain добавляет к ошибке портала её вид и код — по коду удобно искать в документации.
func explain(err error) error {
	var e *bitrix.Error
	if errors.As(err, &e) && e.Code != "" {
		return fmt.Errorf("%s [%s, вид: %s]", e.Text, e.Code, e.Kind())
	}
	return err
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func jsonText(v any) *mcp.CallToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return text("ответ не собрался в JSON: " + err.Error())
	}
	s := string(bitrix.Unslash(b))
	if len(s) > maxText {
		cut := s[:maxText]
		// Не резать посреди буквы UTF-8.
		for len(cut) > 0 && cut[len(cut)-1]&0xC0 == 0x80 {
			cut = cut[:len(cut)-1]
		}
		s = fmt.Sprintf("%s\n…ответ обрезан: показано %d из %d символов. Сузьте выборку: select[] с нужными полями, filter, limit.", cut, len(cut), len(s))
	}
	return text(s)
}
