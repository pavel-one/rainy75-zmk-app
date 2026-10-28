// Package i18n переводит строки приложения. Ключ — русский текст: так код
// остаётся читаемым, а без перевода показывается оригинал.
package i18n

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
)

var english atomic.Bool

func Set(lang string) { english.Store(lang == "en") }

func Lang() string {
	if english.Load() {
		return "en"
	}
	return "ru"
}

// Resolve возвращает язык из настроек, а если он не выбран — язык системы.
func Resolve(lang string) string {
	if lang == "ru" || lang == "en" {
		return lang
	}
	if strings.HasPrefix(systemLocale(), "ru") {
		return "ru"
	}
	return "en"
}

func systemLocale() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" && v != "C" && v != "POSIX" {
			return strings.ToLower(v)
		}
	}
	// Приложения из Finder запускаются без LANG.
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output(); err == nil {
			return strings.ToLower(strings.TrimSpace(string(out)))
		}
	}
	return ""
}

func T(ru string) string {
	if english.Load() {
		if s, ok := en[ru]; ok {
			return s
		}
	}
	return ru
}

func Sprintf(ru string, args ...any) string { return fmt.Sprintf(T(ru), args...) }

func Errorf(ru string, args ...any) error { return fmt.Errorf(T(ru), args...) }

// Error — ошибка с переводимым текстом. Текст выбирается при выводе, поэтому
// её можно объявить переменной пакета и сравнивать через errors.Is.
type Error string

func (e Error) Error() string { return T(string(e)) }
