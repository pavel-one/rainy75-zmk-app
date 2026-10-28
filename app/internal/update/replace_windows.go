package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"rainy75/internal/i18n"
)

// replace подменяет exe: запущенный файл Windows не даёт перезаписать, но
// даёт переименовать. Старый удаляется при следующем запуске (Cleanup).
func replace(exe, dir string) error {
	src, err := os.Open(filepath.Join(dir, "rainy75.exe"))
	if err != nil {
		return i18n.Error("в архиве обновления нет приложения")
	}
	defer src.Close()
	old := exe + ".old"
	os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return i18n.Errorf("нет прав на запись в %s — скачайте обновление вручную", filepath.Dir(exe))
	}
	if err := writeFile(exe, src, 0o755); err != nil {
		os.Rename(old, exe)
		return err
	}
	src.Close()
	os.RemoveAll(filepath.Dir(dir))
	script := fmt.Sprintf("Wait-Process -Id %d -ErrorAction SilentlyContinue; Start-Process -FilePath '%s'", os.Getpid(), strings.ReplaceAll(exe, "'", "''"))
	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Start()
}

// Cleanup удаляет exe, оставшийся от прошлого обновления.
func Cleanup() {
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
	}
}
