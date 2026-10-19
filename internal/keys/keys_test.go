package keys

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStore(t *testing.T) {
	t.Setenv("B24_HOME", t.TempDir())
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(""); err == nil {
		t.Fatal("пустой список: ждали ошибку")
	}
	s.Put(Key{Name: "main", Portal: "a.bitrix24.ru"})
	if s.Default != "main" {
		t.Fatal("первый ключ должен стать основным")
	}
	s.Put(Key{Name: "Лаба", Portal: "b.bitrix24.ru"})
	if k, _ := s.Resolve(""); k.Name != "main" {
		t.Fatal("пустое имя — основной ключ")
	}
	if err := s.Rename("MAIN", "work"); err != nil || s.Default != "work" {
		t.Fatalf("rename: %v, основной %q", err, s.Default)
	}
	if err := s.Rename("work", "лаба"); err == nil {
		t.Fatal("переименование в занятое имя (без учёта регистра)")
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	// На Windows прав вида 0600 нет: доступ ограничивает папка профиля.
	info, _ := os.Stat(filepath.Join(os.Getenv("B24_HOME"), "keys.json"))
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("права файла %v", info.Mode().Perm())
	}
	s2, _ := Load()
	if len(s2.Keys) != 2 || s2.Default != "work" {
		t.Fatalf("после чтения: %+v", s2)
	}
	if err := s2.Remove("work"); err != nil || s2.Default != "Лаба" {
		t.Fatalf("remove: %v, основной %q", err, s2.Default)
	}
	for _, bad := range []string{"", "с пробелом", "-начало", "очень-длинное-имя-которое-больше-сорока-знаков"} {
		if ValidName(bad) == nil {
			t.Errorf("имя %q принято", bad)
		}
	}
}
