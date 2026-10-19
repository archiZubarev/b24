//go:build darwin

package clip

import (
	"os/exec"
	"strings"
)

// Copy кладёт текст в буфер (через stdin pbcopy, не через аргументы).
func Copy(s string) error {
	cmd := exec.Command("/usr/bin/pbcopy")
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

// Read — текст из буфера.
func Read() (string, error) {
	out, err := exec.Command("/usr/bin/pbpaste").Output()
	return string(out), err
}

// Clear стирает буфер.
func Clear() error { return Copy("") }
