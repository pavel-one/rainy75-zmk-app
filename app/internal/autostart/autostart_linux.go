package autostart

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// На Linux — ярлык в ~/.config/autostart (стандарт freedesktop).
func desktopPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "rainy75.desktop"), nil
}

func enabled() bool {
	p, err := desktopPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

func enable(exe string) error {
	p, err := desktopPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	quoted := `"` + strings.ReplaceAll(exe, `"`, `\"`) + `"`
	entry := "[Desktop Entry]\nType=Application\nName=Rainy75\nComment=Панель управления Rainy75\n" +
		"Exec=" + quoted + " " + HiddenFlag + "\nX-GNOME-Autostart-enabled=true\n"
	return os.WriteFile(p, []byte(entry), 0o644)
}

func disable() error {
	p, err := desktopPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
