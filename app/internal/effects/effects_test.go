package effects

import (
	"testing"

	"rainy75/internal/rainy"
)

func TestEveryEffectAnimates(t *testing.T) {
	keys := map[string]bool{}
	for _, e := range List {
		if keys[e.Key] {
			t.Fatalf("ключ %q повторяется", e.Key)
		}
		keys[e.Key] = true

		p := Params{Kind: e.Key, Color: rainy.Color{R: 120, G: 140, B: 255}, Color2: rainy.Color{R: 255, G: 60}, Speed: 1, Brightness: 1}
		lit, changed := false, false
		prev := Frame(p, 0)
		for step := 1; step <= 60; step++ {
			f := Frame(p, float64(step)*0.05)
			if len(f) != rainy.Positions {
				t.Fatalf("%s: кадр из %d цветов", e.Key, len(f))
			}
			for i, c := range f {
				if c != (rainy.Color{}) {
					lit = true
				}
				if c != prev[i] {
					changed = true
				}
			}
			prev = f
		}
		if !lit || !changed {
			t.Errorf("%s: горит=%v, меняется=%v", e.Key, lit, changed)
		}
	}
	if len(List) < 20 {
		t.Fatalf("анимаций %d", len(List))
	}
}

func TestBrightnessZeroIsDark(t *testing.T) {
	for _, e := range List {
		for _, c := range Frame(Params{Kind: e.Key, Color: rainy.Color{R: 255, G: 255, B: 255}, Speed: 1}, 1.3) {
			if c.R > 3 || c.G > 3 || c.B > 3 {
				t.Fatalf("%s светится при нулевой яркости: %+v", e.Key, c)
			}
		}
	}
}

func TestPaletteTuning(t *testing.T) {
	base := Frame(Params{Kind: "rainbow", Speed: 1, Brightness: 1}, 0.4)
	shifted := Frame(Params{Kind: "rainbow", Speed: 1, Brightness: 1, HueShift: 120}, 0.4)
	h1, _, _ := toHSV(base[30])
	h2, _, _ := toHSV(shifted[30])
	if d := frac(h2 - h1); d < 0.30 || d > 0.37 {
		t.Fatalf("сдвиг оттенка %.3f, ожидалось ~0.333", d)
	}
	gray := Frame(Params{Kind: "rainbow", Speed: 1, Brightness: 1, Saturation: 0.01}, 0.4)
	if _, s, _ := toHSV(gray[30]); s > 0.05 {
		t.Fatalf("насыщенность %.2f", s)
	}
}

func TestSecondColorUsed(t *testing.T) {
	p := Params{Kind: "candy", Color: rainy.Color{R: 255}, Color2: rainy.Color{B: 255}, Speed: 1, Brightness: 1}
	var red, blue bool
	for _, c := range Frame(p, 0.3) {
		red = red || (c.R > 200 && c.B == 0)
		blue = blue || (c.B > 200 && c.R == 0)
	}
	if !red || !blue {
		t.Fatalf("леденец: красный=%v синий=%v", red, blue)
	}
}

func TestHSVRoundTrip(t *testing.T) {
	for _, c := range []rainy.Color{{R: 255}, {G: 255}, {B: 255}, {R: 124, G: 140, B: 255}, {R: 255, G: 210, B: 26}} {
		h, s, v := toHSV(c)
		got := HSV(h, s, v)
		if d := int(got.R) - int(c.R) + int(got.G) - int(c.G) + int(got.B) - int(c.B); d > 3 || d < -3 {
			t.Fatalf("%+v → %+v", c, got)
		}
	}
}
