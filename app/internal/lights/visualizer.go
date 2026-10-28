package lights

import (
	"rainy75/internal/i18n"
	"time"

	"rainy75/internal/layout"
	"rainy75/internal/rainy"
)

// Bands — сколько полос спектра рисует визуализатор (по ширине клавиатуры).
const Bands = 16

type VisualizerConfig struct {
	Low  string  `json:"low"`  // цвет низа столбика
	High string  `json:"high"` // цвет верха
	Gain float64 `json:"gain"` // чувствительность, 0.5…4
}

func DefaultVisualizer() VisualizerConfig {
	return VisualizerConfig{Low: "#2a6bff", High: "#ff3cb4", Gain: 1.5}
}

func (c VisualizerConfig) Validate() (VisualizerConfig, error) {
	for _, h := range []string{c.Low, c.High} {
		if _, err := ParseColor(h); err != nil {
			return c, err
		}
	}
	if c.Gain <= 0 {
		return c, i18n.Error("чувствительность должна быть больше нуля")
	}
	c.Gain = max(0.5, min(4, c.Gain))
	return c, nil
}

// Visualizer возвращает базовый кадр: столбики спектра, растущие снизу.
// spectrum отдаёт Bands уровней 0…1.
func Visualizer(cfg VisualizerConfig, spectrum func() []float64) Source {
	low, high := mustColor(cfg.Low), mustColor(cfg.High)
	l := layout.Rainy75
	return func(time.Time) []rainy.Color {
		bands := spectrum()
		frame := make([]rainy.Color, rainy.Positions)
		if len(bands) < Bands {
			return frame
		}
		for i, k := range l.Keys {
			band := min(Bands-1, int((k.X+k.W/2)/l.Width*Bands))
			level := clamp01(bands[band] * cfg.Gain)
			fromBottom := 5 - int(k.Y+0.5) // 0 — нижний ряд
			fill := level*6 - float64(fromBottom)
			if fill <= 0 {
				continue
			}
			c := low
			blend(&c, high, float64(fromBottom)/5)
			frame[i] = scale(c, fill)
		}
		return frame
	}
}
