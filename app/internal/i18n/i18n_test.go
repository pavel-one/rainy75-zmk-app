package i18n

import (
	"errors"
	"testing"
)

func TestSwitch(t *testing.T) {
	defer Set("ru")
	err := Error("клавиатура не ответила")
	Set("ru")
	if err.Error() != "клавиатура не ответила" {
		t.Fatal(err)
	}
	Set("en")
	if err.Error() != "the keyboard did not respond" {
		t.Fatal(err)
	}
	if !errors.Is(Errorf("%w: чтение: %v", err, "eof"), err) {
		t.Fatal("Errorf потерял исходную ошибку")
	}
	if got := T("строка без перевода"); got != "строка без перевода" {
		t.Fatal(got)
	}
}

func TestResolve(t *testing.T) {
	t.Setenv("LANG", "ru_RU.UTF-8")
	if Resolve("") != "ru" || Resolve("en") != "en" {
		t.Fatal("ru")
	}
	t.Setenv("LANG", "de_DE.UTF-8")
	if Resolve("") != "en" {
		t.Fatal("en")
	}
}
