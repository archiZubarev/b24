// b24 — ключи Битрикс24 в хранилище системы и вызовы REST ими.
//
// Ссылка входящего вебхука — пароль к порталу. b24 держит её в Связке ключей
// (macOS) или Диспетчере учётных данных (Windows), а наружу отдаёт только по
// прямой команде человека из терминала (b24 copy). Claude пользуется ключами
// через MCP-сервер (b24 mcp) и команду b24 call, не видя самих ссылок.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/archiZubarev/b24/internal/bitrix"
	"github.com/archiZubarev/b24/internal/clip"
	"github.com/archiZubarev/b24/internal/keys"
	"github.com/archiZubarev/b24/internal/mcpserver"
	"github.com/archiZubarev/b24/internal/secret"
	"github.com/archiZubarev/b24/internal/term"
)

var version = "dev"

const usage = `b24 — ключи Битрикс24 и вызовы REST

Ключи
  b24 add <имя> [--clipboard] [--default]   сохранить ключ (ссылку вебхука); ввод скрыт
  b24 import <имя> --file <путь>            перенести ключ из файла (файл не трогается)
  b24 list [--json]                         какие ключи есть (без самих ссылок)
  b24 check [<имя>|--all]                   проверить, что портал принимает ключ
  b24 rename <старое> <новое>               переименовать
  b24 default <имя>                         сделать основным
  b24 remove <имя> [--yes]                  удалить из хранилища
  b24 copy <имя> [--seconds 30]             положить ссылку в буфер (только в терминале)

Вызовы
  b24 call [--key имя] <метод> [ключ=значение ...] [--json '{...}'|@файл] [--all] [--limit N]
      b24 call crm.deal.get id=12
      b24 call crm.deal.list 'filter[STAGE_ID]=WON' 'select[]=ID' 'select[]=TITLE' --all
      (параметры со скобками — в кавычках: иначе zsh примет их за шаблон файлов)
  b24 batch [--key имя] [--halt] имя=метод?параметры ...

Claude
  b24 mcp                                   MCP-сервер для Claude (запускает сам Claude)
  b24 setup                                 подключить к Claude Code серверы b24 и документации
  b24 status                                что установлено и подключено
  b24 version
`

func main() {
	// Отдельное имя записей в хранилище — для проверок, чтобы не задеть
	// настоящие ключи. Вместе с B24_HOME даёт полностью отдельный набор.
	if s := os.Getenv("B24_KEYCHAIN_SERVICE"); s != "" {
		secret.Service = s
	}
	if err := run(os.Args[1:]); err != nil {
		var u usageError
		if errors.As(err, &u) {
			fmt.Fprintln(os.Stderr, "b24:", err)
			fmt.Fprint(os.Stderr, "\n", usage)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "b24:", bitrix.Redact(err.Error()))
		os.Exit(1)
	}
}

type usageError string

func (u usageError) Error() string { return string(u) }

func run(args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "add":
		return cmdAdd(rest)
	case "import":
		return cmdImport(rest)
	case "list", "ls":
		return cmdList(rest)
	case "check":
		return cmdCheck(rest)
	case "rename", "mv":
		return cmdRename(rest)
	case "default":
		return cmdDefault(rest)
	case "remove", "rm":
		return cmdRemove(rest)
	case "copy":
		return cmdCopy(rest)
	case "call":
		return cmdCall(rest)
	case "batch":
		return cmdBatch(rest)
	case "mcp":
		return cmdMCP()
	case "setup":
		return cmdSetup(rest)
	case "status":
		return cmdStatus()
	case "version", "--version", "-v":
		fmt.Println("b24", version)
		return nil
	case "help", "--help", "-h":
		fmt.Print(usage)
		return nil
	case "__clear-clipboard":
		return cmdClearLater(rest)
	}
	return usageError("неизвестная команда " + cmd)
}

// opts — флаги в любом месте строки; остальное — позиционные аргументы.
type opts struct {
	pos   []string
	flags map[string]string
	bools map[string]bool
}

func parse(args []string, withValue, boolean []string) (opts, error) {
	o := opts{flags: map[string]string{}, bools: map[string]bool{}}
	isVal := map[string]bool{}
	for _, f := range withValue {
		isVal[f] = true
	}
	isBool := map[string]bool{}
	for _, f := range boolean {
		isBool[f] = true
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") || a == "--" {
			o.pos = append(o.pos, a)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		switch {
		case isBool[name]:
			o.bools[name] = true
		case isVal[name]:
			if !hasVal {
				if i+1 >= len(args) {
					return o, usageError("у --" + name + " нет значения")
				}
				i++
				val = args[i]
			}
			o.flags[name] = val
		default:
			return o, usageError("неизвестный флаг --" + name)
		}
	}
	return o, nil
}

func ctx() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

// ---------- ключи ----------

func cmdAdd(args []string) error {
	o, err := parse(args, nil, []string{"clipboard", "default", "no-check"})
	if err != nil {
		return err
	}
	if len(o.pos) != 1 {
		return usageError("b24 add <имя>")
	}
	name := o.pos[0]
	if err := keys.ValidName(name); err != nil {
		return err
	}
	var raw string
	switch {
	case o.bools["clipboard"]:
		if raw, err = clip.Read(); err != nil {
			return fmt.Errorf("буфер обмена не прочитался: %w", err)
		}
	case term.IsTerminal(os.Stdin.Fd()):
		fmt.Fprintln(os.Stderr, "Ссылка вебхука — это пароль к порталу. Вставьте её сюда, на экране она не появится.")
		if raw, err = term.ReadSecret("Ссылка вебхука: "); err != nil {
			return err
		}
	default:
		if raw, err = term.ReadSecret(""); err != nil {
			return errors.New("ссылка вебхука не пришла на вход: запустите b24 add в терминале или с --clipboard")
		}
	}
	if err := save(name, raw, o.bools["default"], !o.bools["no-check"]); err != nil {
		return err
	}
	if o.bools["clipboard"] {
		// Ссылка в буфере больше не нужна — не оставляем её там.
		_ = clip.ClearIf(clip.Fingerprint(raw))
	}
	return nil
}

func cmdImport(args []string) error {
	o, err := parse(args, []string{"file"}, []string{"default", "no-check"})
	if err != nil {
		return err
	}
	if len(o.pos) != 1 || o.flags["file"] == "" {
		return usageError("b24 import <имя> --file <путь>")
	}
	if err := keys.ValidName(o.pos[0]); err != nil {
		return err
	}
	path := expandHome(o.flags["file"])
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("файл %s не прочитался: %w", path, err)
	}
	if err := save(o.pos[0], string(b), o.bools["default"], !o.bools["no-check"]); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Файл %s не тронут: программы, которые его читают, работают как раньше. Если он больше не нужен — удалите его сами.\n", path)
	return nil
}

// save проверяет ссылку на портале и кладёт её в хранилище.
func save(name, raw string, makeDefault, check bool) error {
	h, err := bitrix.ParseHook(raw)
	if err != nil {
		return err
	}
	k := keys.Key{Name: name, Portal: h.Host, UserID: h.UserID, Added: time.Now()}
	if check {
		c, err := bitrix.New(h.URL)
		if err != nil {
			return err
		}
		cx, cancel := ctx()
		defer cancel()
		if err := describe(cx, c, &k); err != nil {
			return fmt.Errorf("ключ не сохранён: %w", err)
		}
	}
	st, err := keys.Load()
	if err != nil {
		return err
	}
	if old, ok := st.Find(name); ok {
		k.Name, k.Added = old.Name, old.Added
	}
	if err := secret.Set(k.Name, h.URL); err != nil {
		return err
	}
	st.Put(k)
	if makeDefault {
		st.Default = k.Name
	}
	if err := st.Save(); err != nil {
		return err
	}
	fmt.Printf("Ключ «%s» сохранён: %s, %s\n", k.Name, h.Masked(), who(k))
	if len(k.Scopes) > 0 {
		fmt.Printf("Права: %s\n", strings.Join(k.Scopes, ", "))
	}
	if strings.EqualFold(st.Default, k.Name) {
		fmt.Println("Это основной ключ.")
	}
	return nil
}

// describe спрашивает портал, чей это ключ и какие у него права.
func describe(cx context.Context, c *bitrix.Client, k *keys.Key) error {
	r, err := c.Call(cx, "profile", nil)
	if err != nil {
		return err
	}
	var p struct {
		ID       any    `json:"ID"`
		Name     string `json:"NAME"`
		LastName string `json:"LAST_NAME"`
	}
	_ = json.Unmarshal(r.Result, &p)
	k.UserName = strings.TrimSpace(p.Name + " " + p.LastName)
	if s, err := c.Call(cx, "scope", nil); err == nil {
		var sc []string
		if json.Unmarshal(s.Result, &sc) == nil {
			k.Scopes = sc
		}
	}
	k.Checked = time.Now()
	return nil
}

func who(k keys.Key) string {
	if k.UserName != "" {
		return fmt.Sprintf("пользователь %s (ID %d)", k.UserName, k.UserID)
	}
	return fmt.Sprintf("пользователь ID %d", k.UserID)
}

func cmdList(args []string) error {
	o, err := parse(args, nil, []string{"json"})
	if err != nil {
		return err
	}
	st, err := keys.Load()
	if err != nil {
		return err
	}
	if o.bools["json"] {
		b, _ := json.MarshalIndent(st, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if len(st.Keys) == 0 {
		fmt.Println("Ключей нет. Добавьте: b24 add <имя>")
		return nil
	}
	for _, k := range st.Keys {
		mark := " "
		if strings.EqualFold(k.Name, st.Default) {
			mark = "*"
		}
		scopes := strings.Join(k.Scopes, ", ")
		if scopes == "" {
			scopes = "права не проверены"
		}
		fmt.Printf("%s %-16s %-28s %s\n    права: %s\n", mark, k.Name, k.Portal, who(k), scopes)
	}
	fmt.Println("\n* — основной ключ")
	return nil
}

func cmdCheck(args []string) error {
	o, err := parse(args, nil, []string{"all"})
	if err != nil {
		return err
	}
	st, err := keys.Load()
	if err != nil {
		return err
	}
	var targets []*keys.Key
	if o.bools["all"] {
		for i := range st.Keys {
			targets = append(targets, &st.Keys[i])
		}
	} else {
		name := ""
		if len(o.pos) > 0 {
			name = o.pos[0]
		}
		k, err := st.Resolve(name)
		if err != nil {
			return err
		}
		targets = []*keys.Key{k}
	}
	cx, cancel := ctx()
	defer cancel()
	failed := 0
	for _, k := range targets {
		c, err := clientFor(k.Name)
		if err == nil {
			err = describe(cx, c, k)
		}
		if err != nil {
			failed++
			fmt.Printf("✗ %s (%s): %s\n", k.Name, k.Portal, bitrix.Redact(err.Error()))
			continue
		}
		fmt.Printf("✓ %s (%s): %s; права: %s\n", k.Name, k.Portal, who(*k), strings.Join(k.Scopes, ", "))
	}
	if err := st.Save(); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("не прошли проверку: %d", failed)
	}
	return nil
}

func cmdRename(args []string) error {
	o, err := parse(args, nil, nil)
	if err != nil {
		return err
	}
	if len(o.pos) != 2 {
		return usageError("b24 rename <старое> <новое>")
	}
	from, to := o.pos[0], o.pos[1]
	if err := keys.ValidName(to); err != nil {
		return err
	}
	st, err := keys.Load()
	if err != nil {
		return err
	}
	k, ok := st.Find(from)
	if !ok {
		return fmt.Errorf("ключа «%s» нет", from)
	}
	oldName := k.Name
	hook, err := secret.Get(oldName)
	if err != nil {
		return err
	}
	// Сначала новая запись, потом список, потом удаление старой: на любом
	// шаге сбоя ключ не теряется.
	if err := secret.Set(to, hook); err != nil {
		return err
	}
	if err := st.Rename(oldName, to); err != nil {
		return err
	}
	if err := st.Save(); err != nil {
		return err
	}
	if !strings.EqualFold(oldName, to) {
		_ = secret.Delete(oldName)
	}
	fmt.Printf("Ключ «%s» теперь называется «%s».\n", oldName, to)
	return nil
}

func cmdDefault(args []string) error {
	if len(args) != 1 {
		return usageError("b24 default <имя>")
	}
	st, err := keys.Load()
	if err != nil {
		return err
	}
	k, ok := st.Find(args[0])
	if !ok {
		return fmt.Errorf("ключа «%s» нет; есть: %s", args[0], strings.Join(st.Names(), ", "))
	}
	st.Default = k.Name
	if err := st.Save(); err != nil {
		return err
	}
	fmt.Printf("Основной ключ — «%s» (%s).\n", k.Name, k.Portal)
	return nil
}

func cmdRemove(args []string) error {
	o, err := parse(args, nil, []string{"yes"})
	if err != nil {
		return err
	}
	if len(o.pos) != 1 {
		return usageError("b24 remove <имя>")
	}
	st, err := keys.Load()
	if err != nil {
		return err
	}
	k, ok := st.Find(o.pos[0])
	if !ok {
		return fmt.Errorf("ключа «%s» нет", o.pos[0])
	}
	if !o.bools["yes"] {
		if !term.Interactive() {
			return errors.New("удаление без терминала — только с --yes")
		}
		if !term.Confirm(fmt.Sprintf("Удалить ключ «%s» (%s)? Сам вебхук в Битриксе останется", k.Name, k.Portal)) {
			fmt.Println("Не удалён.")
			return nil
		}
	}
	name := k.Name
	if err := secret.Delete(name); err != nil && !errors.Is(err, secret.ErrNotFound) {
		return err
	}
	if err := st.Remove(name); err != nil {
		return err
	}
	if err := st.Save(); err != nil {
		return err
	}
	fmt.Printf("Ключ «%s» удалён из хранилища. Чтобы ссылка перестала работать совсем, удалите вебхук в Битриксе.\n", name)
	return nil
}

func cmdCopy(args []string) error {
	o, err := parse(args, []string{"seconds"}, nil)
	if err != nil {
		return err
	}
	if len(o.pos) > 1 {
		return usageError("b24 copy <имя>")
	}
	// Ключ наружу — только человеку за терминалом. У Claude и скриптов
	// терминала нет, и эта команда им откажет.
	if !term.Interactive() {
		return errors.New("b24 copy выдаёт ключ только человеку в терминале; Claude и скрипты пользуются ключом через b24 call и b24 mcp")
	}
	secs := 30
	if s := o.flags["seconds"]; s != "" {
		if secs, err = strconv.Atoi(s); err != nil || secs < 5 || secs > 600 {
			return usageError("--seconds — от 5 до 600")
		}
	}
	st, err := keys.Load()
	if err != nil {
		return err
	}
	name := ""
	if len(o.pos) == 1 {
		name = o.pos[0]
	}
	k, err := st.Resolve(name)
	if err != nil {
		return err
	}
	hook, err := secret.Get(k.Name)
	if err != nil {
		return err
	}
	if err := clip.Copy(hook); err != nil {
		return err
	}
	if err := clearLater(clip.Fingerprint(hook), secs); err != nil {
		fmt.Fprintln(os.Stderr, "Внимание: отложенная очистка не запустилась, очистите буфер сами:", err)
	}
	fmt.Printf("Ссылка ключа «%s» в буфере обмена на %d с. Не вставляйте её в чаты и письма.\n", k.Name, secs)
	return nil
}

func cmdClearLater(args []string) error {
	if len(args) != 2 {
		return nil
	}
	secs, _ := strconv.Atoi(args[1])
	time.Sleep(time.Duration(secs) * time.Second)
	return clip.ClearIf(args[0])
}

// ---------- вызовы ----------

func clientFor(name string) (*bitrix.Client, error) {
	c, _, err := open(name)
	return c, err
}

func open(name string) (*bitrix.Client, *keys.Key, error) {
	st, err := keys.Load()
	if err != nil {
		return nil, nil, err
	}
	k, err := st.Resolve(name)
	if err != nil {
		return nil, nil, err
	}
	hook, err := secret.Get(k.Name)
	if errors.Is(err, secret.ErrNotFound) {
		return nil, nil, fmt.Errorf("ключ «%s» есть в списке, но его нет в хранилище — добавьте заново: b24 add %s", k.Name, k.Name)
	}
	if err != nil {
		return nil, nil, err
	}
	c, err := bitrix.New(hook)
	return c, k, err
}

func cmdCall(args []string) error {
	o, err := parse(args, []string{"key", "json", "limit"}, []string{"all", "raw"})
	if err != nil {
		return err
	}
	if len(o.pos) == 0 {
		return usageError("b24 call <метод> [ключ=значение ...]")
	}
	method := o.pos[0]
	params, err := bitrix.ParseArgs(o.pos[1:])
	if err != nil {
		return err
	}
	if j := o.flags["json"]; j != "" {
		jp, err := bitrix.ParseJSON(j)
		if err != nil {
			return err
		}
		params = bitrix.Merge(params, jp)
	}
	c, _, err := open(o.flags["key"])
	if err != nil {
		return err
	}
	cx, cancel := ctx()
	defer cancel()
	if o.bools["all"] {
		limit := 0
		if s := o.flags["limit"]; s != "" {
			if limit, err = strconv.Atoi(s); err != nil || limit < 0 {
				return usageError("--limit — число строк")
			}
		}
		items, total, err := c.ListAll(cx, method, params, limit)
		if err != nil {
			return err
		}
		printJSON(items)
		fmt.Fprintf(os.Stderr, "строк: %d", len(items))
		if total >= 0 {
			fmt.Fprintf(os.Stderr, " из %d", total)
		}
		fmt.Fprintln(os.Stderr)
		return nil
	}
	var p any
	if len(params) > 0 {
		p = params
	}
	r, err := c.Call(cx, method, p)
	if err != nil {
		return err
	}
	if o.bools["raw"] {
		printJSON(r)
		return nil
	}
	printJSON(r.Result)
	if r.Total != nil {
		fmt.Fprintf(os.Stderr, "всего: %d", *r.Total)
		if r.Next != nil {
			fmt.Fprintf(os.Stderr, ", следующая страница: start=%d (или --all)", *r.Next)
		}
		fmt.Fprintln(os.Stderr)
	}
	return nil
}

func cmdBatch(args []string) error {
	o, err := parse(args, []string{"key", "json"}, []string{"halt"})
	if err != nil {
		return err
	}
	cmds := map[string]string{}
	for _, a := range o.pos {
		name, cmd, ok := strings.Cut(a, "=")
		if !ok {
			return usageError("команда пачки — имя=метод?параметры")
		}
		cmds[name] = cmd
	}
	if j := o.flags["json"]; j != "" {
		m, err := bitrix.ParseJSON(j)
		if err != nil {
			return err
		}
		for k, v := range m {
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("команда %s — не строка «метод?параметры»", k)
			}
			cmds[k] = s
		}
	}
	if len(cmds) == 0 {
		return usageError("b24 batch имя=метод?параметры ...")
	}
	c, _, err := open(o.flags["key"])
	if err != nil {
		return err
	}
	cx, cancel := ctx()
	defer cancel()
	r, err := c.Batch(cx, cmds, o.bools["halt"])
	if err != nil {
		return err
	}
	printJSON(r)
	return nil
}

func printJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Println(v)
		return
	}
	fmt.Println(string(bitrix.Unslash(b)))
}

func cmdMCP() error {
	cx, cancel := ctx()
	defer cancel()
	return mcpserver.Run(cx, version, open, keys.Load)
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return h + p[1:]
		}
	}
	return p
}
