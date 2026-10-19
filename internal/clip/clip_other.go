//go:build !darwin && !windows

package clip

import "errors"

var errNoClipboard = errors.New("буфер обмена поддерживается только на macOS и Windows")

func Copy(s string) error   { return errNoClipboard }
func Read() (string, error) { return "", errNoClipboard }
func Clear() error          { return errNoClipboard }
