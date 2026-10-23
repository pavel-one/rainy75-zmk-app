// Package device находит последовательный порт Rainy75 с открытой прошивкой
// ZMK (rainy75-studio) и открывает на нём SMP-клиент.
package device

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"

	"rainy75/internal/smp"
)

// zmkVendorID — ZMK по умолчанию представляется идентификаторами OpenMoko.
const zmkVendorID = "1D50"

var ErrNotFound = errors.New("Rainy75 не найдена: подключите её кабелем USB-C. По Bluetooth управление недоступно")

type Info struct {
	Port string `json:"port"`
	VID  string `json:"vid"`
	PID  string `json:"pid"`
}

// candidateName — порт CDC ACM, когда USB-идентификаторы недоступны.
func candidateName(name string) bool {
	base := filepath.Base(name)
	return strings.HasPrefix(base, "cu.usbmodem") || strings.HasPrefix(base, "ttyACM")
}

func Find() ([]Info, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, fmt.Errorf("список последовательных портов: %w", err)
	}
	var found []Info
	for _, p := range ports {
		// На macOS у порта два имени: tty.* ждёт сигнала DCD, нужен cu.*.
		if strings.HasPrefix(filepath.Base(p.Name), "tty.") {
			continue
		}
		zmk := p.IsUSB && strings.EqualFold(p.VID, zmkVendorID)
		if zmk || candidateName(p.Name) {
			found = append(found, Info{Port: p.Name, VID: strings.ToUpper(p.VID), PID: strings.ToUpper(p.PID)})
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		return found[i].VID == zmkVendorID && found[j].VID != zmkVendorID
	})
	return found, nil
}

func Open(info Info) (*smp.Client, error) {
	p, err := serial.Open(info.Port, &serial.Mode{BaudRate: 115200})
	if err != nil {
		var pe *serial.PortError
		if errors.As(err, &pe) && pe.Code() == serial.PortBusy {
			return nil, fmt.Errorf("порт %s занят другой программой (Rainy75 Studio, mcumgr или консоль) — закройте её", info.Port)
		}
		return nil, fmt.Errorf("открытие порта %s: %w", info.Port, err)
	}
	return smp.NewClient(p), nil
}
