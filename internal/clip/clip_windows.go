//go:build windows

package clip

import (
	"errors"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Буфер обмена Windows прямыми вызовами user32: без PowerShell и clip.exe,
// чтобы ключ не проходил через чужие процессы.
var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	emptyClipboard   = user32.NewProc("EmptyClipboard")
	getClipboardData = user32.NewProc("GetClipboardData")
	setClipboardData = user32.NewProc("SetClipboardData")
	globalAlloc      = kernel32.NewProc("GlobalAlloc")
	globalFree       = kernel32.NewProc("GlobalFree")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// open — буфер бывает занят другой программой: пробуем полсекунды.
func open() error {
	runtime.LockOSThread()
	for i := 0; i < 25; i++ {
		if r, _, _ := openClipboard.Call(0); r != 0 {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	runtime.UnlockOSThread()
	return errors.New("буфер обмена занят другой программой")
}

func closeClip() {
	closeClipboard.Call()
	runtime.UnlockOSThread()
}

// Copy кладёт текст в буфер.
func Copy(s string) error {
	if err := open(); err != nil {
		return err
	}
	defer closeClip()
	if r, _, e := emptyClipboard.Call(); r == 0 {
		return e
	}
	if s == "" {
		return nil
	}
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return err
	}
	size := uintptr(len(u) * 2)
	h, _, e := globalAlloc.Call(gmemMoveable, size)
	if h == 0 {
		return e
	}
	p, _, e := globalLock.Call(h)
	if p == 0 {
		globalFree.Call(h)
		return e
	}
	copy(unsafe.Slice((*uint16)(ptr(p)), len(u)), u)
	globalUnlock.Call(h)
	if r, _, e := setClipboardData.Call(cfUnicodeText, h); r == 0 {
		globalFree.Call(h)
		return e
	}
	return nil
}

// Read — текст из буфера.
func Read() (string, error) {
	if err := open(); err != nil {
		return "", err
	}
	defer closeClip()
	h, _, _ := getClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return "", nil
	}
	p, _, e := globalLock.Call(h)
	if p == 0 {
		return "", e
	}
	defer globalUnlock.Call(h)
	return windows.UTF16PtrToString((*uint16)(ptr(p))), nil
}

// ptr — адрес памяти, которую вернул GlobalLock. Память системная и не
// двигается сборщиком мусора, поэтому переход от uintptr к указателю здесь
// безопасен; так же это делает golang.org/x/sys.
func ptr(u uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&u)) }

// Clear стирает буфер.
func Clear() error { return Copy("") }
