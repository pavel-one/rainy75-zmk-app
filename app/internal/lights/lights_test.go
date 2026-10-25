package lights

import (
	"testing"
	"time"

	"rainy75/internal/effects"
	"rainy75/internal/indicator"
	"rainy75/internal/rainy"
)

type sink struct {
	sent, cleared int
	last          []rainy.Color
	mirror        []rainy.Color
}

func newEngine() (*Engine, *sink) {
	s := &sink{}
	e := New(
		func(f []rainy.Color) error { s.sent++; s.last = f; return nil },
		func() error { s.cleared++; return nil },
		func(f []rainy.Color) { s.mirror = f },
	)
	return e, s
}

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func TestEngineSendsOnlyChanges(t *testing.T) {
	e, s := newEngine()
	e.Tick(t0)
	if s.sent != 0 || s.cleared != 0 {
		t.Fatalf("эффекты прошивки: отправлено %d, сброшено %d", s.sent, s.cleared)
	}
	frame := make([]rainy.Color, rainy.Positions)
	frame[5] = rainy.Color{R: 10}
	e.SetCanvas(frame)
	for i := range 5 {
		e.Tick(t0.Add(time.Duration(i) * Period))
	}
	if s.sent != 1 || s.last[5].R != 10 || s.mirror == nil {
		t.Fatalf("неизменный холст отправлен %d раз", s.sent)
	}
	e.SetFirmware()
	e.Tick(t0.Add(time.Second))
	if s.cleared != 1 || s.mirror != nil {
		t.Fatalf("возврат к прошивке: сброшено %d", s.cleared)
	}
	e.SetAnimation(effects.Params{Kind: "rainbow", Speed: 1, Brightness: 1})
	for i := range 4 {
		e.Tick(t0.Add(2*time.Second + time.Duration(i)*Period))
	}
	if s.sent != 5 {
		t.Fatalf("анимация: отправлено %d кадров", s.sent-1)
	}
}

func TestLayerOverFirmwareThenReturns(t *testing.T) {
	e, s := newEngine()
	mode := &ModeLayer{}
	e.AddLayer(mode)
	cfg := indicator.Default()
	cfg.Duration = 500
	mode.Trigger(cfg, true, t0)
	e.Tick(t0.Add(200 * time.Millisecond))
	if s.sent != 1 || s.last[40] == (rainy.Color{}) {
		t.Fatalf("индикация не показана: %+v", s.last)
	}
	e.Tick(t0.Add(time.Second))
	if s.cleared != 1 {
		t.Fatal("после индикации клавиатура должна вернуться к эффектам прошивки")
	}
}

func TestHostLostSwitchesToFirmware(t *testing.T) {
	e, s := newEngine()
	e.SetCanvas(make([]rainy.Color, rainy.Positions))
	e.Tick(t0)
	e.HostLost()
	if b, _ := e.State(); b != BaseFirmware {
		t.Fatalf("база %s", b)
	}
	e.Tick(t0.Add(Period))
	if s.sent != 1 {
		t.Fatal("после Fn-клавиши подсветки кадры слать не нужно")
	}
}

func TestReactiveRipple(t *testing.T) {
	cfg := DefaultReactive()
	cfg.Enabled = true
	r := NewReactive(cfg)
	frame := make([]rainy.Color, rainy.Positions)
	if r.Render(t0, frame) {
		t.Fatal("без нажатий слой неактивен")
	}
	r.Press(76, t0) // пробел
	frame = make([]rainy.Color, rainy.Positions)
	if !r.Render(t0.Add(100*time.Millisecond), frame) || frame[76] == (rainy.Color{}) {
		t.Fatal("нажатая клавиша должна гореть")
	}
	if r.Render(t0.Add(2*time.Second), make([]rainy.Color, rainy.Positions)) {
		t.Fatal("след нажатия должен погаснуть")
	}
}

func TestMeterFillsNumberRow(t *testing.T) {
	m := &MeterLayer{}
	cfg := DefaultMeter()
	cfg.Enabled, cfg.Mode = true, "always"
	m.SetConfig(cfg)
	m.Set(0.55, t0)
	frame := make([]rainy.Color, rainy.Positions)
	for i := range frame {
		frame[i] = rainy.Color{R: 100, G: 100, B: 100}
	}
	m.Render(t0, frame)
	if frame[16] == (rainy.Color{R: 100, G: 100, B: 100}) || frame[20] == frame[21] {
		t.Fatalf("шкала: %+v %+v %+v", frame[16], frame[20], frame[21])
	}
	if frame[25].R > 20 {
		t.Fatalf("пустая часть шкалы должна быть тёмной: %+v", frame[25])
	}
}

func TestCapsAndLang(t *testing.T) {
	c := &CapsLayer{}
	c.SetConfig(CapsConfig{Enabled: true, Keys: "caps", Color: "#ffffff"})
	frame := make([]rainy.Color, rainy.Positions)
	if c.Render(t0, frame) {
		t.Fatal("Caps Lock выключен — слой неактивен")
	}
	c.Set(true)
	c.Render(t0, frame)
	if frame[posCaps] != (rainy.Color{R: 255, G: 255, B: 255}) {
		t.Fatalf("Caps: %+v", frame[posCaps])
	}

	l := &LangLayer{}
	cfg := DefaultLang()
	cfg.Enabled, cfg.ShowFor = true, 1000
	l.SetConfig(cfg)
	l.Set("ru", t0)
	frame = make([]rainy.Color, rainy.Positions)
	l.Render(t0.Add(500*time.Millisecond), frame)
	if frame[posEsc] != mustColor(cfg.RuColor) {
		t.Fatalf("RU: %+v", frame[posEsc])
	}
	if l.Render(t0.Add(2*time.Second), make([]rainy.Color, rainy.Positions)) {
		t.Fatal("цвет раскладки должен пропасть через showFor")
	}
}

func TestPomodoroPhases(t *testing.T) {
	flash := &FlashLayer{}
	phases := make(chan string, 4)
	p := NewPomodoro(PomodoroConfig{Work: 1, Break: 1, Keys: "frow", WorkColor: "#ff0000", BreakColor: "#00ff00"},
		flash, func(st PomodoroState) { phases <- st.Phase })
	st := p.Start(t0)
	if st.Phase != "work" || st.Left != 60 {
		t.Fatalf("старт: %+v", st)
	}
	frame := make([]rainy.Color, rainy.Positions)
	p.Render(t0.Add(30*time.Second), frame)
	if frame[1].R < 200 || frame[12].R > 30 {
		t.Fatalf("половина работы: F1=%+v F12=%+v", frame[1], frame[12])
	}
	st = p.Pause(t0.Add(30 * time.Second))
	if st.Running || st.Left != 30 {
		t.Fatalf("пауза: %+v", st)
	}
	p.Start(t0.Add(time.Hour)) // после паузы осталось 30 с
	p.Render(t0.Add(time.Hour+31*time.Second), make([]rainy.Color, rainy.Positions))
	if got := <-phases; got != "break" {
		t.Fatalf("фаза %s", got)
	}
	if !flash.Render(t0.Add(time.Hour+31*time.Second+100*time.Millisecond), make([]rainy.Color, rainy.Positions)) {
		t.Fatal("смена фазы должна мигнуть")
	}
	if st := p.Stop(t0); st.Phase != "idle" {
		t.Fatalf("сброс: %+v", st)
	}
}

func TestConfigValidation(t *testing.T) {
	if _, err := (ReactiveConfig{Style: "boom", Color: "#ffffff"}).Validate(); err == nil {
		t.Fatal("неизвестный стиль")
	}
	if _, err := (CapsConfig{Keys: "caps", Color: "white"}).Validate(); err == nil {
		t.Fatal("цвет не #rrggbb")
	}
	c, err := (PomodoroConfig{Work: 0, Break: 999, Keys: "frow", WorkColor: "#ffffff", BreakColor: "#000000"}).Validate()
	if err != nil || c.Work != 1 || c.Break != 60 {
		t.Fatalf("%+v %v", c, err)
	}
}
