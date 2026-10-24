package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Rainy75", "settings.json")
	if got := Load(path); got != Default() {
		t.Fatalf("без файла: %+v", got)
	}
	s := Default()
	s.Reactive.Enabled, s.Reactive.Style = true, "heat"
	s.Pomodoro.Work = 50
	s.Bluetooth = false
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); got != s {
		t.Fatalf("прочитано %+v", got)
	}
}

func TestBrokenSectionKeepsOthers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{
		"reactive": {"enabled": true, "style": "boom", "color": "#ffffff", "duration": 500},
		"caps": {"enabled": true, "keys": "letters", "color": "#00ff00"},
		"pomodoro": "мусор"
	}`), 0o644)
	got := Load(path)
	if got.Reactive != Default().Reactive {
		t.Fatalf("испорченная секция reactive: %+v", got.Reactive)
	}
	if !got.Caps.Enabled || got.Caps.Keys != "letters" {
		t.Fatalf("исправная секция caps потерялась: %+v", got.Caps)
	}
	if got.Pomodoro != Default().Pomodoro {
		t.Fatalf("pomodoro: %+v", got.Pomodoro)
	}
}

func TestIndicatorFileFromPreviousVersion(t *testing.T) {
	// Прошлая версия хранила в файле только секцию indicator.
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"indicator": {"enabled": false, "onHardware": true, "style": "flash",
		"keys": "esc", "macColor": "#ffffff", "winColor": "#0078d4", "duration": 800}}`), 0o644)
	got := Load(path)
	if got.Indicator.Enabled || got.Indicator.Style != "flash" || got.Indicator.Keys != "esc" {
		t.Fatalf("indicator: %+v", got.Indicator)
	}
}
