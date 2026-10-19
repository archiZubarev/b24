//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// clearLater запускает отдельный процесс, который сотрёт буфер через secs
// секунд, если в нём всё ещё ключ. Процесс отвязан от терминала: закрытие
// окна его не прерывает. В аргументах — только отпечаток, не сам ключ.
func clearLater(fingerprint string, secs int) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "__clear-clipboard", fingerprint, strconv.Itoa(secs))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
