//go:build probe

package main

import (
	"testing"
	"time"

	"rainy75/internal/device"
	"rainy75/internal/effects"
	"rainy75/internal/rainy"
)

func openProbe(t *testing.T) *rainy.Keyboard {
	t.Helper()
	ports, err := device.Find()
	if err != nil || len(ports) == 0 {
		t.Fatalf("порты: %v %v", ports, err)
	}
	t.Logf("порты: %+v", ports)
	c, err := device.Open(ports[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return rainy.New(c)
}

// Только чтение: go test -tags probe -run TestProbeRead -v
func TestProbeRead(t *testing.T) {
	kb := openProbe(t)
	start := time.Now()
	info, err := kb.Info()
	t.Logf("Info: %+v err=%v (%v)", info, err, time.Since(start))
	win, err := kb.Windows()
	t.Logf("Windows: %v err=%v", win, err)
	imgs, err := kb.Images()
	for _, img := range imgs {
		t.Logf("слот %d: v%s active=%v confirmed=%v pending=%v bootable=%v hash=%x",
			img.Slot, img.Version, img.Active, img.Confirmed, img.Pending, img.Bootable, img.Hash[:min(8, len(img.Hash))])
	}
	if err != nil {
		t.Logf("Images err=%v", err)
	}
}

// Подсветка на пару секунд, затем возврат к эффектам: go test -tags probe -run TestProbeLight -v
func TestProbeLight(t *testing.T) {
	kb := openProbe(t)
	defer func() { t.Logf("Clear: %v", kb.Clear()) }()
	t.Logf("Fill: %v", kb.Fill(rainy.Color{R: 0, G: 40, B: 120}))
	time.Sleep(time.Second)
	t.Logf("SetKeys: %v", kb.SetKeys(map[int]rainy.Color{0: {255, 0, 0}, 14: {0, 255, 0}, 76: {255, 255, 255}, 82: {255, 0, 255}}))
	time.Sleep(time.Second)
	p := effects.Params{Kind: "rainbow", Speed: 1, Brightness: 0.6}
	start := time.Now()
	n := 0
	for time.Since(start) < 2*time.Second {
		if err := kb.Frame(effects.Frame(p, time.Since(start).Seconds())); err != nil {
			t.Fatalf("кадр %d: %v", n, err)
		}
		n++
	}
	t.Logf("кадров за 2 с: %d (%.1f к/с)", n, float64(n)/time.Since(start).Seconds())
	info, err := kb.Info()
	t.Logf("Info в режиме компьютера: %+v err=%v", info, err)
}
