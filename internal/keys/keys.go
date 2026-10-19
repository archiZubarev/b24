// Пакет keys — список ключей: имя, портал, пользователь, права. Сами ссылки
// вебхуков здесь не хранятся — они в хранилище системы (пакет secret), а этот
// файл можно показывать кому угодно.
package keys

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Key — сведения о ключе без самой ссылки.
type Key struct {
	Name     string    `json:"name"`
	Portal   string    `json:"portal"`
	UserID   int       `json:"user_id"`
	UserName string    `json:"user_name,omitempty"`
	Scopes   []string  `json:"scopes,omitempty"`
	Added    time.Time `json:"added"`
	Checked  time.Time `json:"checked,omitempty"`
}

// Store — файл keys.json в папке настроек.
type Store struct {
	Default string `json:"default,omitempty"`
	Keys    []Key  `json:"keys"`
	path    string
}

// Dir — папка настроек: B24_HOME или <настройки пользователя>/b24
// (~/Library/Application Support/b24, %AppData%\b24).
func Dir() (string, error) {
	if d := os.Getenv("B24_HOME"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "b24"), nil
}

// Load читает список; файла нет — пустой список.
func Load() (*Store, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "keys.json")}
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, s); err != nil {
		return nil, fmt.Errorf("список ключей %s повреждён: %w", s.path, err)
	}
	return s, nil
}

// Save пишет список атомарно, файл доступен только владельцу.
func (s *Store) Save() error {
	sort.Slice(s.Keys, func(i, j int) bool { return s.Keys[i].Name < s.Keys[j].Name })
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Path — где лежит список.
func (s *Store) Path() string { return s.path }

var nameRe = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}._-]{0,39}$`)

// ValidName — имя ключа: буквы, цифры, точка, дефис, подчёркивание, до 40 знаков.
func ValidName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("имя «%s» не подходит: буквы, цифры, точка, дефис и подчёркивание, без пробелов, до 40 знаков", name)
	}
	return nil
}

// Find — ключ по имени (без учёта регистра).
func (s *Store) Find(name string) (*Key, bool) {
	for i := range s.Keys {
		if strings.EqualFold(s.Keys[i].Name, name) {
			return &s.Keys[i], true
		}
	}
	return nil, false
}

// Resolve — ключ по имени; пустое имя — ключ по умолчанию, а если ключ один — он.
func (s *Store) Resolve(name string) (*Key, error) {
	if name == "" {
		name = s.Default
	}
	if name == "" {
		switch len(s.Keys) {
		case 0:
			return nil, errors.New("ключей ещё нет — добавьте: b24 add <имя>")
		case 1:
			return &s.Keys[0], nil
		}
		return nil, fmt.Errorf("ключей несколько (%s) — укажите имя или выберите основной: b24 default <имя>", strings.Join(s.Names(), ", "))
	}
	k, ok := s.Find(name)
	if !ok {
		return nil, fmt.Errorf("ключа «%s» нет; есть: %s", name, strings.Join(s.Names(), ", "))
	}
	return k, nil
}

// Names — имена по порядку.
func (s *Store) Names() []string {
	out := make([]string, len(s.Keys))
	for i, k := range s.Keys {
		out[i] = k.Name
	}
	sort.Strings(out)
	return out
}

// Put добавляет ключ или заменяет одноимённый.
func (s *Store) Put(k Key) {
	if old, ok := s.Find(k.Name); ok {
		k.Name = old.Name
		*old = k
		return
	}
	s.Keys = append(s.Keys, k)
	if len(s.Keys) == 1 {
		s.Default = k.Name
	}
}

// Rename меняет имя ключа в списке.
func (s *Store) Rename(from, to string) error {
	k, ok := s.Find(from)
	if !ok {
		return fmt.Errorf("ключа «%s» нет", from)
	}
	if other, ok := s.Find(to); ok && other != k {
		return fmt.Errorf("ключ «%s» уже есть", to)
	}
	if strings.EqualFold(s.Default, k.Name) {
		s.Default = to
	}
	k.Name = to
	return nil
}

// Remove убирает ключ из списка.
func (s *Store) Remove(name string) error {
	for i, k := range s.Keys {
		if strings.EqualFold(k.Name, name) {
			s.Keys = append(s.Keys[:i], s.Keys[i+1:]...)
			if strings.EqualFold(s.Default, name) {
				s.Default = ""
				if len(s.Keys) == 1 {
					s.Default = s.Keys[0].Name
				}
			}
			return nil
		}
	}
	return fmt.Errorf("ключа «%s» нет", name)
}
