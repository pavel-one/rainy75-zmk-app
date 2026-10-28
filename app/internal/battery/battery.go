// Package battery читает заряд Rainy75 из операционной системы.
//
// По кабелю (SMP) прошивка заряд не отдаёт, зато по Bluetooth клавиатура
// публикует стандартный Battery Service, который ОС выставляет в реестр
// устройств (IORegistry на macOS, power_supply на Linux). Имя «Rainy 75 Pro»
// у открытой прошивки, VendorID 0x320F — у заводской.
package battery

import (
	"context"
	"rainy75/internal/i18n"
	"regexp"
	"strconv"
	"strings"
)

type Status struct {
	Available bool   `json:"available"`
	Percent   int    `json:"percent"`
	Charging  bool   `json:"charging"`
	Source    string `json:"source"`
	Message   string `json:"message"`
}

const vendorIDDecimal = "12815" // 0x320F

func Read(ctx context.Context) Status { return readPlatform(ctx) }

func notFound() Status {
	return Status{
		Message: i18n.T("ОС не сообщает заряд. Подключите клавиатуру по Bluetooth или нажмите Fn+B — индикатор на клавишах покажет уровень."),
	}
}

func isRainy(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "rainy") || strings.Contains(n, "wobkey")
}

var (
	reProduct  = regexp.MustCompile(`"Product" = "([^"]*)"`)
	reVendor   = regexp.MustCompile(`"VendorID" = (\d+)`)
	rePercent  = regexp.MustCompile(`"BatteryPercent" = (\d+)`)
	reCharging = regexp.MustCompile(`"BatteryStatusFlags" = (\d+)`)
)

// parseIORegistry разбирает вывод `ioreg -r -l -k BatteryPercent`.
func parseIORegistry(out string) (Status, bool) {
	for _, block := range strings.Split(out, "+-o ") {
		p := rePercent.FindStringSubmatch(block)
		if p == nil {
			continue
		}
		prod := reProduct.FindStringSubmatch(block)
		vend := reVendor.FindStringSubmatch(block)
		if !(prod != nil && isRainy(prod[1])) && !(vend != nil && vend[1] == vendorIDDecimal) {
			continue
		}
		pct, _ := strconv.Atoi(p[1])
		s := Status{Available: true, Percent: min(pct, 100), Source: "Bluetooth (macOS)"}
		// Бит 0x2 в BatteryStatusFlags означает зарядку.
		if c := reCharging.FindStringSubmatch(block); c != nil {
			flags, _ := strconv.Atoi(c[1])
			s.Charging = flags&0x2 != 0
		}
		return s, true
	}
	return Status{}, false
}
