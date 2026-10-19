package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/archiZubarev/b24/internal/keys"
)

// Серверы, которые подключает b24 setup.
const (
	docsName = "b24-dev-mcp"
	docsURL  = "https://mcp-dev.bitrix24.tech/mcp"
	selfName = "b24"
)

// cmdSetup подключает к Claude Code (на уровне пользователя, для всех папок)
// MCP-сервер документации Битрикса и MCP-сервер b24.
func cmdSetup(args []string) error {
	o, err := parse(args, nil, []string{"print"})
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if p, err := filepath.EvalSymlinks(self); err == nil {
		self = p
	}
	steps := [][]string{
		{"mcp", "add", "--scope", "user", "--transport", "http", docsName, docsURL},
		{"mcp", "add", "--scope", "user", selfName, "--", self, "mcp"},
	}
	claude, err := exec.LookPath("claude")
	if o.bools["print"] || err != nil {
		if err != nil {
			fmt.Println("Команда claude не найдена. Выполните в терминале, где установлен Claude Code:")
		}
		for _, s := range steps {
			fmt.Println("  claude " + quote(s))
		}
		return nil
	}
	for i, s := range steps {
		name := []string{docsName, selfName}[i]
		if exec.Command(claude, "mcp", "get", name).Run() == nil {
			fmt.Printf("✓ %s уже подключён\n", name)
			continue
		}
		cmd := exec.Command(claude, s...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("claude %s: %w", quote(s), err)
		}
		fmt.Printf("✓ %s подключён\n", name)
	}
	fmt.Println("Готово. Перезапустите Claude Code (или откройте новый разговор), чтобы серверы появились.")
	return nil
}

// cmdStatus — что установлено и подключено: по нему скилл понимает, с чего
// начинать у нового человека.
func cmdStatus() error {
	self, _ := os.Executable()
	fmt.Printf("b24 %s: %s\n", version, self)
	st, err := keys.Load()
	if err != nil {
		fmt.Println("ключи: список не прочитался:", err)
	} else if len(st.Keys) == 0 {
		fmt.Println("ключи: нет — добавьте: b24 add <имя>")
	} else {
		fmt.Printf("ключи: %d (%s), основной: %s\n", len(st.Keys), strings.Join(st.Names(), ", "), orDash(st.Default))
	}
	claude, err := exec.LookPath("claude")
	if err != nil {
		fmt.Println("claude: не найден в PATH — MCP-серверы не проверить")
	} else {
		for _, name := range []string{docsName, selfName} {
			if exec.Command(claude, "mcp", "get", name).Run() == nil {
				fmt.Printf("mcp %s: подключён\n", name)
			} else {
				fmt.Printf("mcp %s: не подключён — b24 setup\n", name)
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		skill := filepath.Join(home, ".claude", "skills", "bitrix24", "SKILL.md")
		if _, err := os.Stat(skill); err == nil {
			fmt.Println("скилл bitrix24: установлен")
		} else {
			fmt.Println("скилл bitrix24: не найден в ~/.claude/skills/bitrix24")
		}
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func quote(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"'") {
			a = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}
