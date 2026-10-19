// Пакет term — ввод без эха и проверка, что команду запустил человек в
// терминале. Команды, которые отдают ключ наружу (copy), работают только из
// терминала: у Claude и скриптов терминала нет, и ключ им не достаётся.
package term

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// Interactive — и ввод, и вывод идут в терминал.
func Interactive() bool {
	return IsTerminal(os.Stdin.Fd()) && IsTerminal(os.Stdout.Fd())
}

// ReadSecret печатает подсказку и читает строку без эха. Вне терминала читает
// строку из stdin как есть.
func ReadSecret(prompt string) (string, error) {
	os.Stderr.WriteString(prompt)
	fd := os.Stdin.Fd()
	if !IsTerminal(fd) {
		return readLine(os.Stdin)
	}
	restore, err := noEcho(fd)
	if err != nil {
		return "", err
	}
	defer func() {
		restore()
		os.Stderr.WriteString("\n")
	}()
	return readLine(os.Stdin)
}

// Confirm задаёт вопрос «да/нет» в терминале.
func Confirm(question string) bool {
	os.Stderr.WriteString(question + " [д/н]: ")
	s, _ := readLine(os.Stdin)
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "д" || s == "да" || s == "y" || s == "yes"
}

// stdin — один буфер на процесс: новый буфер на каждый вопрос съедал бы
// строки, прочитанные про запас прошлым.
var stdin = bufio.NewReader(os.Stdin)

func readLine(r io.Reader) (string, error) {
	br := stdin
	if r != os.Stdin {
		br = bufio.NewReader(r)
	}
	line, err := br.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
