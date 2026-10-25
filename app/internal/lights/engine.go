// Package lights собирает кадр подсветки из базы (эффекты прошивки, холст,
// анимация, визуализатор) и слоёв поверх неё (реакция на нажатия,
// индикаторы, вспышки) и отправляет его в клавиатуру, только если он изменился.
//
// Прошивка показывает либо свои эффекты, либо кадры с компьютера. Поэтому
// слой поверх эффектов прошивки рисуется на тёмном фоне, а когда ни базы, ни
// активных слоёв нет, клавиатура возвращается к своим эффектам.
package lights

import (
	"context"
	"slices"
	"sync"
	"time"

	"rainy75/internal/effects"
	"rainy75/internal/rainy"
)

const Period = 50 * time.Millisecond

type Base string

const (
	BaseFirmware   Base = "firmware"   // эффекты прошивки
	BaseCanvas     Base = "canvas"     // холст, нарисованный в приложении
	BaseAnimation  Base = "animation"  // анимация из internal/effects
	BaseVisualizer Base = "visualizer" // визуализатор звука
)

// Layer — слой поверх базы. Render дорисовывает кадр и сообщает, активен ли
// слой сейчас (неактивный слой кадр не трогает).
type Layer interface {
	Render(now time.Time, frame []rainy.Color) bool
}

type Source func(now time.Time) []rainy.Color

type Engine struct {
	mu        sync.Mutex
	base      Base
	canvas    []rainy.Color
	anim      effects.Params
	animStart time.Time
	source    Source
	layers    []Layer

	send    func([]rainy.Color) error // отправить кадр в клавиатуру
	clear   func() error              // вернуть эффекты прошивки
	onFrame func([]rainy.Color)       // кадр для схемы в интерфейсе; nil — кадров нет

	last []rainy.Color // последний отправленный кадр
	host bool          // клавиатура показывает кадры с компьютера
}

// New создаёт компоновщик. Кадры уходят через send, возврат к эффектам
// прошивки — через clear.
func New(send func([]rainy.Color) error, clear func() error, onFrame func([]rainy.Color)) *Engine {
	return &Engine{base: BaseFirmware, canvas: make([]rainy.Color, rainy.Positions), send: send, clear: clear, onFrame: onFrame}
}

func (e *Engine) AddLayer(l Layer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.layers = append(e.layers, l)
}

func (e *Engine) State() (Base, effects.Params) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.base, e.anim
}

func (e *Engine) SetFirmware() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.base = BaseFirmware
}

func (e *Engine) SetCanvas(frame []rainy.Color) {
	e.mu.Lock()
	defer e.mu.Unlock()
	copy(e.canvas, frame)
	e.base = BaseCanvas
}

func (e *Engine) SetKeys(keys map[int]rainy.Color) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for pos, c := range keys {
		if pos >= 0 && pos < rainy.Positions {
			e.canvas[pos] = c
		}
	}
	e.base = BaseCanvas
}

func (e *Engine) SetAnimation(p effects.Params) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.base != BaseAnimation || e.anim.Kind != p.Kind {
		e.animStart = time.Now()
	}
	e.anim, e.base = p, BaseAnimation
}

func (e *Engine) SetSource(src Source) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.source, e.base = src, BaseVisualizer
}

// Resync заставляет заново отправить кадр: клавиатура переподключилась.
func (e *Engine) Resync() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.last, e.host = nil, false
}

// HostLost вызывается, когда прошивка сама вышла из режима кадров
// (нажали Fn-клавишу подсветки): пользователь выбрал эффекты прошивки.
func (e *Engine) HostLost() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.host {
		return
	}
	if e.base != BaseVisualizer {
		e.base = BaseFirmware
	}
	e.last, e.host = nil, false
}

func (e *Engine) Run(ctx context.Context) {
	t := time.NewTicker(Period)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			e.Tick(now)
		}
	}
}

func (e *Engine) Tick(now time.Time) {
	e.mu.Lock()
	frame, needHost := e.composeLocked(now)
	host, last := e.host, e.last
	e.mu.Unlock()

	if !needHost {
		if host && e.clear() == nil {
			e.mu.Lock()
			e.host, e.last = false, nil
			e.mu.Unlock()
			e.onFrame(nil)
		}
		return
	}
	if host && slices.Equal(frame, last) {
		return
	}
	if e.send(frame) != nil {
		return // нет связи — повторим на следующем кадре
	}
	e.mu.Lock()
	e.host, e.last = true, frame
	e.mu.Unlock()
	e.onFrame(frame)
}

func (e *Engine) composeLocked(now time.Time) ([]rainy.Color, bool) {
	frame := make([]rainy.Color, rainy.Positions)
	need := true
	switch e.base {
	case BaseCanvas:
		copy(frame, e.canvas)
	case BaseAnimation:
		copy(frame, effects.Frame(e.anim, now.Sub(e.animStart).Seconds()))
	case BaseVisualizer:
		if e.source != nil {
			copy(frame, e.source(now))
		}
	default:
		need = false
	}
	for _, l := range e.layers {
		if l.Render(now, frame) {
			need = true
		}
	}
	return frame, need
}
