//go:build windows

package secret

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Диспетчер учётных данных Windows: обычная учётная запись с адресом
// «b24:<имя ключа>». Ключ шифрует сама система под пользователем Windows.
var (
	advapi32   = windows.NewLazySystemDLL("advapi32.dll")
	credWrite  = advapi32.NewProc("CredWriteW")
	credRead   = advapi32.NewProc("CredReadW")
	credDelete = advapi32.NewProc("CredDeleteW")
	credFree   = advapi32.NewProc("CredFree")
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
)

// credential — структура CREDENTIALW из wincred.h.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func target(name string) (*uint16, error) {
	return windows.UTF16PtrFromString(Service + ":" + name)
}

// Set кладёт ключ под именем name; запись с тем же именем заменяется.
func Set(name, value string) error {
	if value == "" {
		return fmt.Errorf("пустой ключ")
	}
	t, err := target(name)
	if err != nil {
		return err
	}
	user, _ := windows.UTF16PtrFromString(name)
	comment, _ := windows.UTF16PtrFromString("Битрикс24 — ключ b24")
	blob := []byte(value)
	defer clear(blob)
	c := credential{
		Type:               credTypeGeneric,
		TargetName:         t,
		Comment:            comment,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
		UserName:           user,
	}
	r, _, e := credWrite.Call(uintptr(unsafe.Pointer(&c)), 0)
	if r == 0 {
		return fmt.Errorf("Диспетчер учётных данных не сохранил ключ: %v", e)
	}
	return nil
}

// Get достаёт ключ по имени.
func Get(name string) (string, error) {
	t, err := target(name)
	if err != nil {
		return "", err
	}
	var p *credential
	r, _, e := credRead.Call(uintptr(unsafe.Pointer(t)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		if errors.Is(e, windows.ERROR_NOT_FOUND) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("Диспетчер учётных данных не отдал ключ: %v", e)
	}
	defer credFree.Call(uintptr(unsafe.Pointer(p)))
	return string(unsafe.Slice(p.CredentialBlob, p.CredentialBlobSize)), nil
}

// Delete удаляет ключ из хранилища.
func Delete(name string) error {
	t, err := target(name)
	if err != nil {
		return err
	}
	r, _, e := credDelete.Call(uintptr(unsafe.Pointer(t)), credTypeGeneric, 0)
	if r == 0 {
		if errors.Is(e, windows.ERROR_NOT_FOUND) {
			return ErrNotFound
		}
		return fmt.Errorf("Диспетчер учётных данных не удалил ключ: %v", e)
	}
	return nil
}
