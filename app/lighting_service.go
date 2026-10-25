package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"rainy75/internal/effects"
	"rainy75/internal/host"
	"rainy75/internal/indicator"
	"rainy75/internal/layout"
	"rainy75/internal/lights"
	"rainy75/internal/rainy"
	"rainy75/internal/settings"
)

type KeyColor struct {
	Pos int   `json:"pos"`
	R   uint8 `json:"r"`
	G   uint8 `json:"g"`
	B   uint8 `json:"b"`
}

type AnimationParams struct {
	Kind       string  `json:"kind"`
	Color      string  `json:"color"`  // #rrggbb
	Color2     string  `json:"color2"` // второй цвет двухцветных эффектов
	Speed      float64 `json:"speed"`
	Brightness float64 `json:"brightness"`
	HueShift   float64 `json:"hueShift"`   // сдвиг оттенка палитры, градусы
	Saturation float64 `json:"saturation"` // насыщенность палитры, 0…1
}

type LightingState struct {
	Base      string          `json:"base"` // firmware | canvas | animation | visualizer
	Animation AnimationParams `json:"animation"`
}

type Permissions struct {
	Supported       bool `json:"supported"`       // системные данные есть только на macOS
	InputMonitoring bool `json:"inputMonitoring"` // перехват нажатий
}

type lighting struct {
	engine   *lights.Engine
	mode     *lights.ModeLayer
	flash    *lights.FlashLayer
	reactive *lights.ReactiveLayer
	caps     *lights.CapsLayer
	lang     *lights.LangLayer
	meter    *lights.MeterLayer
	pomodoro *lights.PomodoroLayer

	keyTap    atomic.Bool // перехват нажатий запущен
	codeToPos [2]map[string]int

	audioMu  sync.Mutex
	audioOn  bool
	spectrum *host.Spectrum

	animMu sync.Mutex
	anim   AnimationParams
}

func (s *KeyboardService) initLighting() {
	cfg := s.cfg
	s.mode = &lights.ModeLayer{}
	s.flash = &lights.FlashLayer{}
	s.reactive = lights.NewReactive(cfg.Reactive)
	s.caps = &lights.CapsLayer{}
	s.caps.SetConfig(cfg.Caps)
	s.lang = &lights.LangLayer{}
	s.lang.SetConfig(cfg.Lang)
	s.meter = &lights.MeterLayer{}
	s.meter.SetConfig(cfg.Meter)
	s.pomodoro = lights.NewPomodoro(cfg.Pomodoro, s.flash, func(st lights.PomodoroState) { emit(eventPomodoro, st) })

	s.engine = lights.New(s.sendFrame, s.clearFrame, s.onFrame)
	// Снизу вверх: постоянные индикаторы, затем события, сверху — вспышки.
	for _, l := range []lights.Layer{s.meter, s.lang, s.caps, s.pomodoro, s.reactive, s.flash, s.mode} {
		s.engine.AddLayer(l)
	}

	// Нажатие → позиция keymap: в режиме macOS ⌥ и ⌘ стоят на других местах.
	for i, mac := range []bool{true, false} {
		m := map[string]int{}
		for _, k := range layout.Rainy75.Keys {
			code := k.Code
			if mac {
				code = k.MacCode
			}
			if code != "" {
				m[code] = k.Pos
			}
		}
		s.codeToPos[i] = m
	}
}

func (s *KeyboardService) startLighting(ctx context.Context) {
	go s.engine.Run(ctx)
	go s.watchSystem(ctx)
}

func (s *KeyboardService) stopLighting() {
	s.stopAudio()
	host.StopKeyTap()
	if s.Status().Host {
		_ = s.clearFrame()
	}
}

func (s *KeyboardService) sendFrame(frame []rainy.Color) error {
	return s.withKeyboard(func(kb *rainy.Keyboard) error { return kb.Frame(frame) })
}

func (s *KeyboardService) clearFrame() error {
	return s.withKeyboard(func(kb *rainy.Keyboard) error { return kb.Clear() })
}

func (s *KeyboardService) onFrame(frame []rainy.Color) {
	if frame == nil {
		// Типизированный nil: событие зарегистрировано как []string.
		emit(eventFrame, []string(nil))
	} else {
		out := make([]string, len(frame))
		for i, c := range frame {
			out[i] = fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
		}
		emit(eventFrame, out)
	}
	s.mu.Lock()
	changed := s.status.Connected && s.status.Host != (frame != nil)
	s.status.Host = frame != nil
	st := s.status
	s.mu.Unlock()
	if changed {
		emit(eventDevice, st)
	}
}

func (s *KeyboardService) watchSystem(ctx context.Context) {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	var cpu host.CPU
	var tick int
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			tick++
			cfg := s.config()
			if cfg.Caps.Enabled {
				s.caps.Set(host.CapsLock())
			}
			if cfg.Lang.Enabled && tick%2 == 0 {
				if lang := host.InputLanguage(); lang != "" {
					s.lang.Set(lang, now)
				}
			}
			if cfg.Meter.Enabled {
				switch {
				case cfg.Meter.Source == "volume":
					if v, muted, err := host.Volume(); err == nil {
						if muted {
							v = 0
						}
						s.meter.Set(v, now)
					}
				case tick%5 == 0: // загрузку процессора меряем по секунде
					if v, err := cpu.Usage(); err == nil {
						s.meter.Set(v, now)
					}
				}
			}
			if tick%10 == 0 {
				s.syncKeyTap(cfg.Reactive.Enabled)
			}
		}
	}
}

// syncKeyTap включает перехват нажатий, когда нужна реакция на них.
// Разрешение могут выдать позже — поэтому вызывается периодически.
func (s *KeyboardService) syncKeyTap(want bool) {
	switch {
	case want && !s.keyTap.Load():
		if host.StartKeyTap(s.onKey) == nil {
			s.keyTap.Store(true)
		}
	case !want && s.keyTap.Load():
		host.StopKeyTap()
		s.keyTap.Store(false)
	}
}

func (s *KeyboardService) onKey(code string) {
	m := s.codeToPos[0]
	if s.windows.Load() {
		m = s.codeToPos[1]
	}
	if pos, ok := m[code]; ok {
		s.reactive.Press(pos, time.Now())
	}
}

func (s *KeyboardService) emitLighting() { emit(eventLighting, s.LightingState()) }

func (s *KeyboardService) LightingState() LightingState {
	base, _ := s.engine.State()
	s.animMu.Lock()
	defer s.animMu.Unlock()
	return LightingState{Base: string(base), Animation: s.anim}
}

func (s *KeyboardService) requireConnected() error {
	if st := s.Status(); !st.Connected {
		return errors.New(st.Message)
	}
	return nil
}

func (s *KeyboardService) FillColor(r, g, b uint8) error {
	frame := make([]rainy.Color, rainy.Positions)
	for i := range frame {
		frame[i] = rainy.Color{R: r, G: g, B: b}
	}
	return s.setCanvas(frame)
}

func (s *KeyboardService) SetFrame(frame []KeyColor) error {
	colors := make([]rainy.Color, rainy.Positions)
	for _, k := range frame {
		if k.Pos < 0 || k.Pos >= rainy.Positions {
			return fmt.Errorf("нет клавиши %d", k.Pos)
		}
		colors[k.Pos] = rainy.Color{R: k.R, G: k.G, B: k.B}
	}
	return s.setCanvas(colors)
}

func (s *KeyboardService) setCanvas(frame []rainy.Color) error {
	if err := s.requireConnected(); err != nil {
		return err
	}
	s.stopAudio()
	s.engine.SetCanvas(frame)
	s.emitLighting()
	return nil
}

// RestoreEffects возвращает эффекты прошивки (их переключают клавиши Fn).
// Включённые индикаторы продолжают рисоваться поверх тёмного фона.
func (s *KeyboardService) RestoreEffects() error {
	s.stopAudio()
	s.engine.SetFirmware()
	s.emitLighting()
	return nil
}

func (s *KeyboardService) StartAnimation(p AnimationParams) error {
	if !effects.Valid(p.Kind) {
		return fmt.Errorf("неизвестная анимация %q", p.Kind)
	}
	c1, err := lights.ParseColor(p.Color)
	if err != nil {
		return err
	}
	c2, err := lights.ParseColor(p.Color2)
	if err != nil {
		return err
	}
	if err := s.requireConnected(); err != nil {
		return err
	}
	s.stopAudio()
	s.engine.SetAnimation(effects.Params{
		Kind: p.Kind, Color: c1, Color2: c2, Speed: p.Speed, Brightness: p.Brightness,
		HueShift: p.HueShift, Saturation: p.Saturation,
	})
	s.animMu.Lock()
	s.anim = p
	s.animMu.Unlock()
	s.emitLighting()
	return nil
}

func (s *KeyboardService) StopAnimation() error { return s.RestoreEffects() }

func (s *KeyboardService) StartVisualizer() error {
	if err := s.requireConnected(); err != nil {
		return err
	}
	s.audioMu.Lock()
	if !s.audioOn {
		if err := host.StartAudio(); err != nil {
			s.audioMu.Unlock()
			return err
		}
		s.audioOn, s.spectrum = true, host.NewSpectrum(lights.Bands)
	}
	s.audioMu.Unlock()
	s.engine.SetSource(lights.Visualizer(s.config().Visualizer, s.readSpectrum))
	s.emitLighting()
	return nil
}

func (s *KeyboardService) readSpectrum() []float64 {
	s.audioMu.Lock()
	defer s.audioMu.Unlock()
	if !s.audioOn {
		return nil
	}
	samples, rate := host.AudioSamples(2048)
	return s.spectrum.Update(samples, rate)
}

func (s *KeyboardService) stopAudio() {
	s.audioMu.Lock()
	defer s.audioMu.Unlock()
	if s.audioOn {
		host.StopAudio()
		s.audioOn = false
	}
}

func (s *KeyboardService) Settings() settings.Settings { return s.config() }

func (s *KeyboardService) SetSettings(c settings.Settings) (settings.Settings, error) {
	c, err := c.Validate()
	if err != nil {
		return s.config(), err
	}
	if s.cfgPath != "" {
		if err := settings.Save(s.cfgPath, c); err != nil {
			return s.config(), fmt.Errorf("сохранение настроек: %w", err)
		}
	}
	s.cfgMu.Lock()
	old := s.cfg
	s.cfg = c
	s.cfgMu.Unlock()

	s.reactive.SetConfig(c.Reactive)
	s.caps.SetConfig(c.Caps)
	s.lang.SetConfig(c.Lang)
	s.meter.SetConfig(c.Meter)
	s.pomodoro.SetConfig(c.Pomodoro)
	go s.syncKeyTap(c.Reactive.Enabled)
	if base, _ := s.engine.State(); base == lights.BaseVisualizer && c.Visualizer != old.Visualizer {
		s.engine.SetSource(lights.Visualizer(c.Visualizer, s.readSpectrum))
	}
	// Bluetooth выключили — закрываем соединение по нему.
	if old.Bluetooth && !c.Bluetooth {
		s.mu.Lock()
		if s.status.Connection == "bluetooth" {
			s.disconnectLocked("Управление по Bluetooth выключено")
		}
		st := s.status
		s.mu.Unlock()
		emit(eventDevice, st)
	}
	return c, nil
}

func (s *KeyboardService) Permissions() Permissions {
	return Permissions{Supported: host.Supported, InputMonitoring: host.KeyTapAllowed()}
}

func (s *KeyboardService) RequestInputMonitoring() bool { return host.RequestKeyTap() }

// PreviewIndicator показывает индикацию режима ("mac" или "windows") с
// переданными настройками, даже если они ещё не сохранены.
func (s *KeyboardService) PreviewIndicator(mode string, c indicator.Config) error {
	c, err := c.Validate()
	if err != nil {
		return err
	}
	if err := s.requireConnected(); err != nil {
		return err
	}
	s.mode.Trigger(c, mode == "windows", time.Now())
	return nil
}

// Flash мигает всей клавиатурой: например, по команде `rainy75 --flash`.
func (s *KeyboardService) Flash(color string, times int) error {
	c, err := lights.ParseColor(color)
	if err != nil {
		return err
	}
	if err := s.requireConnected(); err != nil {
		return err
	}
	s.flash.Trigger(c, times, time.Now())
	return nil
}

func (s *KeyboardService) pomodoroChanged(st lights.PomodoroState) lights.PomodoroState {
	emit(eventPomodoro, st)
	return st
}

func (s *KeyboardService) PomodoroState() lights.PomodoroState { return s.pomodoro.State(time.Now()) }

func (s *KeyboardService) PomodoroStart() lights.PomodoroState {
	return s.pomodoroChanged(s.pomodoro.Start(time.Now()))
}

func (s *KeyboardService) PomodoroPause() lights.PomodoroState {
	return s.pomodoroChanged(s.pomodoro.Pause(time.Now()))
}

func (s *KeyboardService) PomodoroStop() lights.PomodoroState {
	return s.pomodoroChanged(s.pomodoro.Stop(time.Now()))
}

func (s *KeyboardService) PomodoroSkip() lights.PomodoroState {
	return s.pomodoroChanged(s.pomodoro.Skip(time.Now()))
}
