//go:build darwin || windows

package secret

import (
	"errors"
	"os"
	"testing"
)

// Пишет в настоящее хранилище системы, но под отдельным сервисом b24-test
// и удаляет за собой. Запуск: B24_KEYSTORE_TEST=1 go test ./internal/secret.
func TestRoundTrip(t *testing.T) {
	if os.Getenv("B24_KEYSTORE_TEST") != "1" {
		t.Skip("B24_KEYSTORE_TEST=1 — тест пишет в Связку ключей / Диспетчер учётных данных")
	}
	Service = "b24-test"
	name := "проверка-1"
	t.Cleanup(func() { _ = Delete(name) })
	if _, err := Get(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("до записи: %v", err)
	}
	if err := Set(name, "https://example.bitrix24.ru/rest/1/aaa/"); err != nil {
		t.Fatal(err)
	}
	if err := Set(name, "https://example.bitrix24.ru/rest/1/bbb/"); err != nil {
		t.Fatal("перезапись:", err)
	}
	got, err := Get(name)
	if err != nil || got != "https://example.bitrix24.ru/rest/1/bbb/" {
		t.Fatalf("чтение: %q, %v", got, err)
	}
	if err := Delete(name); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("после удаления: %v", err)
	}
}
