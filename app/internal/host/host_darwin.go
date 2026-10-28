// Package host даёт подсветке данные системы: нажатия клавиш, раскладку,
// Caps Lock, загрузку процессора, громкость и звук. Работает на macOS.
package host

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=12.0
#cgo LDFLAGS: -framework Foundation -framework ApplicationServices -framework Carbon -framework CoreAudio -framework AudioToolbox
#include <stdlib.h>
#include "host_darwin.h"
*/
import "C"

import (
	"os/exec"
	"rainy75/internal/i18n"
	"sync"
	"unsafe"
)

const Supported = true

var (
	keyMu    sync.Mutex
	keyFunc  func(code string)
	keyStart error
	keyOnce  sync.Once
)

//export rhGoKey
func rhGoKey(vk C.int) {
	code, ok := keyCodes[int(vk)]
	if !ok {
		return
	}
	keyMu.Lock()
	f := keyFunc
	keyMu.Unlock()
	if f != nil {
		f(code)
	}
}

func KeyTapAllowed() bool { return C.rh_keytap_allowed() != 0 }

// inputMonitoringPane — раздел «Мониторинг ввода» в Системных настройках.
const inputMonitoringPane = "x-apple.systempreferences:com.apple.preference.security?Privacy_ListenEvent"

// RequestKeyTap просит разрешение. Свой запрос macOS показывает только один
// раз, поэтому без разрешения сразу открываем нужный раздел Настроек.
func RequestKeyTap() bool {
	if C.rh_keytap_request() != 0 {
		return true
	}
	_ = exec.Command("open", inputMonitoringPane).Start()
	return false
}

// ErrNoInputMonitoring — нет разрешения «Мониторинг ввода».
var ErrNoInputMonitoring = i18n.Error("нет разрешения «Мониторинг ввода»: Системные настройки → Конфиденциальность и безопасность → Мониторинг ввода")

// StartKeyTap запускает перехват нажатий; onKey получает KeyboardEvent.code.
// Перехват только читает события и ничего не меняет.
func StartKeyTap(onKey func(code string)) error {
	keyMu.Lock()
	keyFunc = onKey
	keyMu.Unlock()
	if !KeyTapAllowed() {
		return ErrNoInputMonitoring
	}
	keyOnce.Do(func() {
		if C.rh_keytap_start() != 0 {
			keyStart = i18n.Error("не удалось перехватить нажатия клавиш")
		}
	})
	return keyStart
}

// StopKeyTap перестаёт передавать нажатия (сам перехват остаётся до выхода).
func StopKeyTap() {
	keyMu.Lock()
	keyFunc = nil
	keyMu.Unlock()
}

// InputLanguage — язык текущей раскладки: «en», «ru»…
// Вызывать не из главного потока: функция сама переходит в него.
func InputLanguage() string {
	p := C.rh_input_language()
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p)
}

func CapsLock() bool { return C.rh_caps_lock() != 0 }

type CPU struct{ busy, total uint64 }

// Usage — доля занятого времени процессора с прошлого вызова, 0…1.
func (c *CPU) Usage() (float64, error) {
	var busy, total C.ulonglong
	if C.rh_cpu_ticks(&busy, &total) != 0 {
		return 0, i18n.Error("не удалось прочитать загрузку процессора")
	}
	b, t := uint64(busy), uint64(total)
	defer func() { c.busy, c.total = b, t }()
	if c.total == 0 || t <= c.total {
		return 0, nil
	}
	return float64(b-c.busy) / float64(t-c.total), nil
}

// Volume — громкость вывода по умолчанию (0…1) и признак «без звука».
func Volume() (float64, bool, error) {
	var v C.float
	var muted C.int
	if rc := C.rh_volume(&v, &muted); rc != 0 {
		return 0, false, i18n.Errorf("не удалось прочитать громкость (код %d)", int(rc))
	}
	return float64(v), muted != 0, nil
}

// StartAudio начинает слушать звук системы (macOS 14.2+).
func StartAudio() error {
	switch rc := C.rh_audio_start(); rc {
	case 0:
		return nil
	case -100:
		return i18n.Error("звук системы доступен с macOS 14.2")
	default:
		return i18n.Errorf("не удалось подключиться к звуку системы (код Core Audio %d); проверьте разрешение «Запись звука системы»", int(rc))
	}
}

func StopAudio() { C.rh_audio_stop() }

func AudioSamples(n int) ([]float32, float64) {
	buf := make([]float32, n)
	if n > 0 {
		C.rh_audio_read((*C.float)(unsafe.Pointer(&buf[0])), C.int(n))
	}
	return buf, float64(C.rh_audio_rate())
}
