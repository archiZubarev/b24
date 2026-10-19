//go:build darwin

package secret

/*
#cgo LDFLAGS: -framework CoreFoundation -framework Security
#include <stdlib.h>
#include <string.h>
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>

static CFStringRef b24Str(const char *s) {
	return CFStringCreateWithCString(kCFAllocatorDefault, s, kCFStringEncodingUTF8);
}

// b24Query — запрос к записи «обычный пароль» с нашим сервисом и именем ключа.
static CFMutableDictionaryRef b24Query(const char *service, const char *account) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef svc = b24Str(service), acc = b24Str(account);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, svc);
	CFDictionarySetValue(q, kSecAttrAccount, acc);
	CFRelease(svc);
	CFRelease(acc);
	return q;
}

static OSStatus b24Set(const char *service, const char *account, const void *data, int n) {
	CFMutableDictionaryRef q = b24Query(service, account);
	CFDataRef value = CFDataCreate(kCFAllocatorDefault, data, n);
	CFMutableDictionaryRef upd = CFDictionaryCreateMutable(kCFAllocatorDefault, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFDictionarySetValue(upd, kSecValueData, value);
	OSStatus st = SecItemUpdate(q, upd);
	if (st == errSecItemNotFound) {
		CFStringRef label = b24Str("Битрикс24 — ключ b24");
		CFDictionarySetValue(q, kSecValueData, value);
		CFDictionarySetValue(q, kSecAttrLabel, label);
		CFRelease(label);
		st = SecItemAdd(q, NULL);
	}
	CFRelease(upd);
	CFRelease(value);
	CFRelease(q);
	return st;
}

static OSStatus b24Get(const char *service, const char *account, CFDataRef *out) {
	CFMutableDictionaryRef q = b24Query(service, account);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	OSStatus st = SecItemCopyMatching(q, (CFTypeRef *)out);
	CFRelease(q);
	return st;
}

static OSStatus b24Delete(const char *service, const char *account) {
	CFMutableDictionaryRef q = b24Query(service, account);
	OSStatus st = SecItemDelete(q);
	CFRelease(q);
	return st;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const (
	errSecItemNotFound = -25300
	errSecUserCanceled = -128
	errSecAuthFailed   = -25293
)

// Set кладёт ключ под именем name; запись с тем же именем заменяется.
func Set(name, value string) error {
	if value == "" {
		return fmt.Errorf("пустой ключ")
	}
	svc, acc := C.CString(Service), C.CString(name)
	defer C.free(unsafe.Pointer(svc))
	defer C.free(unsafe.Pointer(acc))
	n := len(value)
	p := C.CBytes([]byte(value))
	defer func() {
		// Копию ключа в памяти C затираем перед освобождением.
		C.memset(p, 0, C.size_t(n))
		C.free(p)
	}()
	return status(C.b24Set(svc, acc, p, C.int(n)))
}

// Get достаёт ключ по имени.
func Get(name string) (string, error) {
	svc, acc := C.CString(Service), C.CString(name)
	defer C.free(unsafe.Pointer(svc))
	defer C.free(unsafe.Pointer(acc))
	var data C.CFDataRef
	if err := status(C.b24Get(svc, acc, &data)); err != nil {
		return "", err
	}
	defer C.CFRelease(C.CFTypeRef(data))
	n := C.CFDataGetLength(data)
	return C.GoStringN((*C.char)(unsafe.Pointer(C.CFDataGetBytePtr(data))), C.int(n)), nil
}

// Delete удаляет ключ из хранилища.
func Delete(name string) error {
	svc, acc := C.CString(Service), C.CString(name)
	defer C.free(unsafe.Pointer(svc))
	defer C.free(unsafe.Pointer(acc))
	return status(C.b24Delete(svc, acc))
}

func status(st C.OSStatus) error {
	switch int(st) {
	case 0:
		return nil
	case errSecItemNotFound:
		return ErrNotFound
	case errSecUserCanceled, errSecAuthFailed:
		return fmt.Errorf("доступ к Связке ключей не разрешён")
	}
	return fmt.Errorf("Связка ключей ответила ошибкой %d", int(st))
}
