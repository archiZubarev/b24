// Пакет clip — буфер обмена. Ключ кладётся в буфер только по команде
// человека (b24 copy) и стирается через полминуты, если его не заменили.
package clip

import (
	"crypto/sha256"
	"encoding/hex"
)

// Fingerprint — отпечаток текста: по нему отложенная очистка узнаёт, что
// в буфере всё ещё ключ, не держа сам ключ в аргументах процесса.
func Fingerprint(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// ClearIf стирает буфер, если в нём текст с этим отпечатком.
func ClearIf(fingerprint string) error {
	cur, err := Read()
	if err != nil {
		return err
	}
	if Fingerprint(cur) != fingerprint {
		return nil
	}
	return Clear()
}
