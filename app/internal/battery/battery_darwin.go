package battery

import (
	"context"
	"os/exec"
	"time"
)

func readPlatform(ctx context.Context) Status {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ioreg", "-r", "-l", "-k", "BatteryPercent").Output()
	if err != nil {
		s := notFound()
		s.Message = "Не удалось прочитать IORegistry: " + err.Error()
		return s
	}
	if s, ok := parseIORegistry(string(out)); ok {
		return s
	}
	return notFound()
}
