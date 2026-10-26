// Package autostart добавляет приложение в автозапуск при входе в систему.
// При автозапуске приложение получает флаг HiddenFlag и стартует в трее.
package autostart

import (
	"os"
	"path/filepath"
)

// HiddenFlag — аргумент запуска «сразу в трей, без окна».
const HiddenFlag = "--hidden"

const ID = "one.pavel.rainy75"

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func Enabled() bool { return enabled() }

func Set(on bool) error {
	if !on {
		return disable()
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	return enable(exe)
}
