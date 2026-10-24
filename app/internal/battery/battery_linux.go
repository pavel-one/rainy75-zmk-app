package battery

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func readPlatform(context.Context) Status {
	dirs, _ := filepath.Glob("/sys/class/power_supply/*")
	for _, dir := range dirs {
		model := readFile(filepath.Join(dir, "model_name"))
		if !isRainy(model) && !strings.Contains(strings.ToLower(dir), "320f") {
			continue
		}
		pct, err := strconv.Atoi(readFile(filepath.Join(dir, "capacity")))
		if err != nil {
			continue
		}
		return Status{
			Available: true,
			Percent:   min(pct, 100),
			Charging:  readFile(filepath.Join(dir, "status")) == "Charging",
			Source:    "power_supply (Linux)",
		}
	}
	return notFound()
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
