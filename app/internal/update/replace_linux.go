package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"rainy75/internal/i18n"
)

// replace подменяет бинарник: запущенный файл можно заменить переименованием.
func replace(exe, dir string) error {
	src, err := os.Open(filepath.Join(dir, "rainy75"))
	if err != nil {
		return i18n.Error("в архиве обновления нет приложения")
	}
	defer src.Close()
	tmp := exe + ".new"
	if err := writeFile(tmp, src, 0o755); err != nil {
		return i18n.Errorf("нет прав на запись в %s — скачайте обновление вручную", filepath.Dir(exe))
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return i18n.Errorf("нет прав на запись в %s — скачайте обновление вручную", filepath.Dir(exe))
	}
	os.RemoveAll(filepath.Dir(dir))
	script := fmt.Sprintf("while kill -0 %d 2>/dev/null; do sleep 0.2; done; exec %q", os.Getpid(), exe)
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

func Cleanup() {}
