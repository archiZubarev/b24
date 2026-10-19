//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
)

// clearLater запускает отдельный процесс, который сотрёт буфер через secs
// секунд, если в нём всё ещё ключ. В аргументах — только отпечаток.
func clearLater(fingerprint string, secs int) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "__clear-clipboard", fingerprint, strconv.Itoa(secs))
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | createNewProcessGroup, HideWindow: true}
	return cmd.Start()
}
