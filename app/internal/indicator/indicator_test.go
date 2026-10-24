package indicator

import (
	"testing"
	"time"

	"rainy75/internal/rainy"
)

func TestFramesLightOnlySelectedKeys(t *testing.T) {
	c := Default()
	c.Keys, c.Style, c.Duration = "mods", "solid", 1000
	frames := Frames(c, true, 50*time.Millisecond)
	if len(frames) != 21 {
		t.Fatalf("кадров %d", len(frames))
	}
	mid := frames[10]
	lit := map[int]bool{}
	for i, col := range mid {
		if col != (rainy.Color{}) {
			lit[i] = true
		}
	}
	for _, p := range keySets["mods"] {
		if !lit[p] {
			t.Fatalf("клавиша %d не горит", p)
		}
	}
	if len(lit) != len(keySets["mods"]) {
		t.Fatalf("горят лишние клавиши: %v", lit)
	}
	// Windows — синий из настроек.
	if mid[73] != (rainy.Color{R: 0, G: 0x78, B: 0xd4}) {
		t.Fatalf("цвет Windows: %+v", mid[73])
	}
}

func TestPulseStartsAndEndsDark(t *testing.T) {
	c := Default()
	for _, style := range []string{"pulse", "sweep", "solid", "flash"} {
		c.Style = style
		f := Frames(c, false, 50*time.Millisecond)
		edges := [][]rainy.Color{f[0], f[len(f)-1]}
		if style == "flash" {
			edges = edges[1:] // вспышки начинаются сразу
		}
		for _, edge := range edges {
			for i, col := range edge {
				if col.R > 30 || col.G > 30 || col.B > 30 {
					t.Fatalf("%s: клавиша %d светится на краю: %+v", style, i, col)
				}
			}
		}
	}
}

func TestValidate(t *testing.T) {
	c := Default()
	c.Duration = 99999
	got, err := c.Validate()
	if err != nil || got.Duration != maxDuration {
		t.Fatalf("%+v %v", got, err)
	}
	c.Style = "boom"
	if _, err := c.Validate(); err == nil {
		t.Fatal("неизвестный стиль должен отклоняться")
	}
	c = Default()
	c.MacColor = "red"
	if _, err := c.Validate(); err == nil {
		t.Fatal("цвет не в формате #rrggbb должен отклоняться")
	}
}
