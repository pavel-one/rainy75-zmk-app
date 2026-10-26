//go:build !darwin && !linux && !windows

package autostart

import "errors"

var errUnsupported = errors.New("автозапуск на этой ОС не поддерживается")

func enabled() bool       { return false }
func enable(string) error { return errUnsupported }
func disable() error      { return nil }
