package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"rainy75/internal/i18n"
)

// replace меняет бандл .app целиком: старый откладывается рядом, новый
// копируется через ditto (сохраняет права и подпись).
func replace(exe, dir string) error {
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(bundle, ".app") {
		return i18n.Error("обновлять можно только приложение из .app — эта сборка запущена не из бандла")
	}
	found, _ := filepath.Glob(filepath.Join(dir, "*.app"))
	if len(found) == 0 {
		return i18n.Error("в архиве обновления нет приложения")
	}
	old := filepath.Join(filepath.Dir(bundle), "."+filepath.Base(bundle)+".old")
	os.RemoveAll(old)
	if err := os.Rename(bundle, old); err != nil {
		return i18n.Errorf("нет прав на запись в %s — скачайте обновление вручную", filepath.Dir(bundle))
	}
	if out, err := exec.Command("ditto", found[0], bundle).CombinedOutput(); err != nil {
		os.RemoveAll(bundle)
		os.Rename(old, bundle)
		return fmt.Errorf("ditto: %v: %s", err, out)
	}
	exec.Command("xattr", "-dr", "com.apple.quarantine", bundle).Run()
	os.RemoveAll(old)
	os.RemoveAll(filepath.Dir(dir))
	return relaunch(fmt.Sprintf("open -n %q", bundle))
}

func relaunch(cmd string) error {
	script := fmt.Sprintf("while kill -0 %d 2>/dev/null; do sleep 0.2; done; %s", os.Getpid(), cmd)
	return exec.Command("/bin/sh", "-c", script).Start()
}

func Cleanup() {}
