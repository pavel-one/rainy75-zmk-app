//go:build !darwin && !linux && !windows

package autostart

import (
	"rainy75/internal/i18n"
)

var errUnsupported = i18n.Error("автозапуск на этой ОС не поддерживается")

func enabled() bool       { return false }
func enable(string) error { return errUnsupported }
func disable() error      { return nil }
