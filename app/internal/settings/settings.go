// Package settings хранит настройки приложения в одном JSON-файле.
// Каждая секция проверяется отдельно: испорченная заменяется значениями по
// умолчанию, остальные сохраняются.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"rainy75/internal/i18n"

	"rainy75/internal/indicator"
	"rainy75/internal/lights"
)

type Settings struct {
	Indicator  indicator.Config        `json:"indicator"`
	Reactive   lights.ReactiveConfig   `json:"reactive"`
	Caps       lights.CapsConfig       `json:"caps"`
	Lang       lights.LangConfig       `json:"lang"`
	Meter      lights.MeterConfig      `json:"meter"`
	Pomodoro   lights.PomodoroConfig   `json:"pomodoro"`
	Visualizer lights.VisualizerConfig `json:"visualizer"`
	Bluetooth  bool                    `json:"bluetooth"` // искать клавиатуру и по Bluetooth
	Language   string                  `json:"language"`  // ru, en или пусто — язык системы
}

func Default() Settings {
	return Settings{
		Indicator:  indicator.Default(),
		Reactive:   lights.DefaultReactive(),
		Caps:       lights.DefaultCaps(),
		Lang:       lights.DefaultLang(),
		Meter:      lights.DefaultMeter(),
		Pomodoro:   lights.DefaultPomodoro(),
		Visualizer: lights.DefaultVisualizer(),
		Bluetooth:  true,
	}
}

func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Rainy75", "settings.json"), nil
}

// section разбирает секцию raw, проверяет её и при ошибке оставляет def.
func section[T any](raw json.RawMessage, def T, validate func(T) (T, error)) T {
	if len(raw) == 0 {
		return def
	}
	v := def
	if json.Unmarshal(raw, &v) != nil {
		return def
	}
	v, err := validate(v)
	if err != nil {
		return def
	}
	return v
}

func (s Settings) Validate() (Settings, error) {
	var err error
	if s.Indicator, err = s.Indicator.Validate(); err != nil {
		return s, err
	}
	if s.Reactive, err = s.Reactive.Validate(); err != nil {
		return s, err
	}
	if s.Caps, err = s.Caps.Validate(); err != nil {
		return s, err
	}
	if s.Lang, err = s.Lang.Validate(); err != nil {
		return s, err
	}
	if s.Meter, err = s.Meter.Validate(); err != nil {
		return s, err
	}
	if s.Pomodoro, err = s.Pomodoro.Validate(); err != nil {
		return s, err
	}
	if s.Visualizer, err = s.Visualizer.Validate(); err != nil {
		return s, err
	}
	if s.Language != "" && s.Language != "ru" && s.Language != "en" {
		return s, i18n.Errorf("неизвестный язык %q", s.Language)
	}
	return s, nil
}

// Load читает настройки; чего нет или что испорчено — по умолчанию.
func Load(path string) Settings {
	s := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return s
	}
	s.Indicator = section(raw["indicator"], s.Indicator, indicator.Config.Validate)
	s.Reactive = section(raw["reactive"], s.Reactive, lights.ReactiveConfig.Validate)
	s.Caps = section(raw["caps"], s.Caps, lights.CapsConfig.Validate)
	s.Lang = section(raw["lang"], s.Lang, lights.LangConfig.Validate)
	s.Meter = section(raw["meter"], s.Meter, lights.MeterConfig.Validate)
	s.Pomodoro = section(raw["pomodoro"], s.Pomodoro, lights.PomodoroConfig.Validate)
	s.Visualizer = section(raw["visualizer"], s.Visualizer, lights.VisualizerConfig.Validate)
	s.Bluetooth = section(raw["bluetooth"], s.Bluetooth, func(b bool) (bool, error) { return b, nil })
	s.Language = section(raw["language"], s.Language, func(l string) (string, error) {
		if l != "" && l != "ru" && l != "en" {
			return "", i18n.Errorf("неизвестный язык %q", l)
		}
		return l, nil
	})
	return s
}

// Save записывает настройки через временный файл, чтобы не оставить обрывок.
func Save(path string, s Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
