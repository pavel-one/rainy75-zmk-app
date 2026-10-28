package battery

import (
	"context"
	"os/exec"
	"rainy75/internal/i18n"
	"time"
)

func readPlatform(ctx context.Context) Status {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ioreg", "-r", "-l", "-k", "BatteryPercent").Output()
	if err != nil {
		s := notFound()
		s.Message = i18n.Sprintf("Не удалось прочитать IORegistry: %v", err)
		return s
	}
	if s, ok := parseIORegistry(string(out)); ok {
		return s
	}
	return notFound()
}
