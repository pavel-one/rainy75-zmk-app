package host

import (
	"math"
	"testing"

	"rainy75/internal/layout"
)

func TestSpectrumFindsTone(t *testing.T) {
	const rate = 48000.0
	s := NewSpectrum(16)
	samples := make([]float32, 2048)
	for i := range samples {
		samples[i] = float32(0.5 * math.Sin(2*math.Pi*1000*float64(i)/rate))
	}
	bands := s.Update(samples, rate)
	best := 0
	for b := range bands {
		if bands[b] > bands[best] {
			best = b
		}
	}
	// 1 кГц на шкале 40 Гц…16 кГц из 16 полос попадает в полосу 8.
	if best < 7 || best > 9 || bands[best] < 0.9 {
		t.Fatalf("пик в полосе %d (%.2f): %.2f", best, bands[best], bands)
	}
	quiet := make([]float32, 2048)
	for range 60 {
		bands = s.Update(quiet, rate)
	}
	if bands[best] > 0.05 {
		t.Fatalf("после тишины полоса не погасла: %.2f", bands[best])
	}
}

func TestKeyCodesCoverLayout(t *testing.T) {
	known := map[string]bool{}
	for _, c := range keyCodes {
		known[c] = true
	}
	for _, k := range layout.Rainy75.Keys {
		for _, code := range []string{k.Code, k.MacCode} {
			if code != "" && !known[code] {
				t.Errorf("для %s (%s) нет виртуального кода macOS", code, k.Label)
			}
		}
	}
}

func TestSystemReadings(t *testing.T) {
	if !Supported {
		t.Skip("только macOS")
	}
	var cpu CPU
	if _, err := cpu.Usage(); err != nil {
		t.Fatal(err)
	}
	busy := 0.0
	for range 2_000_000 {
		busy += math.Sqrt(busy + 1)
	}
	u, err := cpu.Usage()
	if err != nil || u < 0 || u > 1 {
		t.Fatalf("загрузка %.2f, ошибка %v", u, err)
	}
	if v, _, err := Volume(); err == nil && (v < 0 || v > 1) {
		t.Fatalf("громкость %.2f", v)
	}
	_ = CapsLock()
}
