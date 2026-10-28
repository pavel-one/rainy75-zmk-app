// Package indicator показывает на подсветке, какой режим включился — macOS
// или Windows: короткая анимация цветом режима на выбранных клавишах.
package indicator

import (
	"fmt"
	"math"
	"rainy75/internal/i18n"
	"regexp"
	"time"

	"rainy75/internal/layout"
	"rainy75/internal/rainy"
)

type Config struct {
	Enabled    bool   `json:"enabled"`
	OnHardware bool   `json:"onHardware"` // показывать и при переключении Fn+M
	Style      string `json:"style"`      // pulse | flash | sweep | solid
	Keys       string `json:"keys"`       // all | mods | frow | esc
	MacColor   string `json:"macColor"`   // #rrggbb
	WinColor   string `json:"winColor"`
	Duration   int    `json:"duration"` // мс
}

// Default — настройки по умолчанию: белый для macOS, синий Windows.
func Default() Config {
	return Config{
		Enabled: true, OnHardware: true, Style: "pulse", Keys: "all",
		MacColor: "#f2f2f7", WinColor: "#0078d4", Duration: 1500,
	}
}

const (
	minDuration = 500
	maxDuration = 5000
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (c Config) Validate() (Config, error) {
	switch c.Style {
	case "pulse", "flash", "sweep", "solid":
	default:
		return c, i18n.Errorf("неизвестный стиль индикации %q", c.Style)
	}
	if _, ok := keySets[c.Keys]; !ok {
		return c, i18n.Errorf("неизвестный набор клавиш %q", c.Keys)
	}
	if !hexColor.MatchString(c.MacColor) || !hexColor.MatchString(c.WinColor) {
		return c, i18n.Error("цвет должен быть в виде #rrggbb")
	}
	c.Duration = max(minDuration, min(maxDuration, c.Duration))
	return c, nil
}

// Наборы клавиш — позиции keymap (см. internal/layout).
var keySets = map[string][]int{
	"all":  nil,                          // все 83
	"mods": {59, 71, 73, 74, 75, 77, 79}, // Shift, Ctrl, Win/⌥, Alt/⌘, правые Alt и Ctrl
	"frow": {0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14},
	"esc":  {0},
}

func parseColor(hex string) rainy.Color {
	var r, g, b uint8
	fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return rainy.Color{R: r, G: g, B: b}
}

func (c Config) Color(win bool) rainy.Color {
	if win {
		return parseColor(c.WinColor)
	}
	return parseColor(c.MacColor)
}

func (c Config) Mask() []bool {
	on := make([]bool, rainy.Positions)
	if set := keySets[c.Keys]; set == nil {
		for i := range on {
			on[i] = true
		}
	} else {
		for _, p := range set {
			on[p] = true
		}
	}
	return on
}

// Frames возвращает кадры индикации режима (win — Windows) с шагом period.
func Frames(c Config, win bool, period time.Duration) [][]rainy.Color {
	color, on := c.Color(win), c.Mask()
	l := layout.Rainy75

	total := time.Duration(c.Duration) * time.Millisecond
	n := max(1, int(total/period))
	frames := make([][]rainy.Color, 0, n+1)
	for f := 0; f <= n; f++ {
		t := float64(f) / float64(n) // 0…1
		frame := make([]rainy.Color, rainy.Positions)
		for i, k := range l.Keys {
			if !on[i] {
				continue
			}
			x := (k.X + k.W/2) / l.Width
			frame[i] = scale(color, Level(c.Style, t, x))
		}
		frames = append(frames, frame)
	}
	return frames
}

// Level — яркость клавиши с координатой x в момент t (оба 0…1).
func Level(style string, t, x float64) float64 {
	fade := min(1, (1-t)*5) // последние 20% — затухание
	switch style {
	case "flash": // три вспышки; последний отрезок (и сам конец) — темнота
		if min(5, int(t*6))%2 == 0 {
			return 1
		}
		return 0
	case "sweep": // волна слева направо, клавиши остаются гореть
		front := t*1.6 - 0.1
		return clamp01((front-x)/0.12) * fade
	case "solid":
		return min(1, t*10) * fade
	default: // pulse
		return math.Sin(math.Pi * t)
	}
}

func clamp01(v float64) float64 { return max(0, min(1, v)) }

func scale(c rainy.Color, l float64) rainy.Color {
	l = clamp01(l)
	return rainy.Color{R: uint8(float64(c.R) * l), G: uint8(float64(c.G) * l), B: uint8(float64(c.B) * l)}
}
