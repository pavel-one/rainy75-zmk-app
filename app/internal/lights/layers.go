package lights

import (
	"fmt"
	"math"
	"rainy75/internal/i18n"
	"strings"
	"sync"
	"time"

	"rainy75/internal/indicator"
	"rainy75/internal/layout"
	"rainy75/internal/rainy"
)

// Наборы клавиш (позиции keymap, см. internal/layout).
var (
	posEsc     = 0
	posCaps    = 44
	posSpace   = 76
	fRow       = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	numberRow  = []int{16, 17, 18, 19, 20, 21, 22, 23, 24, 25} // 1…0
	letterKeys = func() []int {
		var out []int
		for _, k := range layout.Rainy75.Keys {
			if strings.HasPrefix(k.Code, "Key") {
				out = append(out, k.Pos)
			}
		}
		return out
	}()
	// centers — центры клавиш в юнитах.
	centers = func() [][2]float64 {
		out := make([][2]float64, len(layout.Rainy75.Keys))
		for i, k := range layout.Rainy75.Keys {
			out[i] = [2]float64{k.X + k.W/2, k.Y + k.H/2}
		}
		return out
	}()
)

func allKeys() []int {
	out := make([]int, rainy.Positions)
	for i := range out {
		out[i] = i
	}
	return out
}

func keysOf(name string) []int {
	switch name {
	case "esc":
		return []int{posEsc}
	case "caps":
		return []int{posCaps}
	case "space":
		return []int{posSpace}
	case "frow":
		return fRow
	case "numrow":
		return numberRow
	case "letters":
		return letterKeys
	default:
		return allKeys()
	}
}

func validKeys(name string, allowed ...string) error {
	for _, a := range allowed {
		if name == a {
			return nil
		}
	}
	return i18n.Errorf("неизвестный набор клавиш %q", name)
}

func ParseColor(hex string) (rainy.Color, error) {
	var r, g, b uint8
	if len(hex) != 7 || hex[0] != '#' {
		return rainy.Color{}, i18n.Errorf("цвет должен быть в виде #rrggbb, а не %q", hex)
	}
	if _, err := fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b); err != nil {
		return rainy.Color{}, i18n.Errorf("цвет должен быть в виде #rrggbb, а не %q", hex)
	}
	return rainy.Color{R: r, G: g, B: b}, nil
}

func mustColor(hex string) rainy.Color {
	c, _ := ParseColor(hex)
	return c
}

func clamp01(v float64) float64 { return max(0, min(1, v)) }

func blend(dst *rainy.Color, c rainy.Color, a float64) {
	a = clamp01(a)
	m := func(x, y uint8) uint8 { return uint8(float64(x)*(1-a) + float64(y)*a + 0.5) }
	*dst = rainy.Color{R: m(dst.R, c.R), G: m(dst.G, c.G), B: m(dst.B, c.B)}
}

func scale(c rainy.Color, l float64) rainy.Color {
	l = clamp01(l)
	return rainy.Color{R: uint8(float64(c.R) * l), G: uint8(float64(c.G) * l), B: uint8(float64(c.B) * l)}
}

func hsv(h, s, v float64) rainy.Color {
	h -= math.Floor(h)
	i := math.Floor(h * 6)
	f := h*6 - i
	p, q, t := v*(1-s), v*(1-f*s), v*(1-(1-f)*s)
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return rainy.Color{R: uint8(r * 255), G: uint8(g * 255), B: uint8(b * 255)}
}

type ModeLayer struct {
	mu    sync.Mutex
	until time.Time
	start time.Time
	cfg   indicator.Config
	win   bool
}

func (l *ModeLayer) Trigger(cfg indicator.Config, win bool, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cfg, l.win, l.start = cfg, win, now
	l.until = now.Add(time.Duration(cfg.Duration) * time.Millisecond)
}

func (l *ModeLayer) Active(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return now.Before(l.until)
}

func (l *ModeLayer) Render(now time.Time, frame []rainy.Color) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !now.Before(l.until) {
		return false
	}
	t := float64(now.Sub(l.start)) / float64(l.until.Sub(l.start))
	color, mask := l.cfg.Color(l.win), l.cfg.Mask()
	w := layout.Rainy75.Width
	for i, on := range mask {
		if on {
			blend(&frame[i], color, indicator.Level(l.cfg.Style, t, centers[i][0]/w))
		}
	}
	return true
}

// FlashLayer мигает всей клавиатурой: уведомления, конец помодоро.
type FlashLayer struct {
	mu    sync.Mutex
	start time.Time
	until time.Time
	color rainy.Color
	times int
}

const flashPeriod = 400 * time.Millisecond

func (l *FlashLayer) Trigger(c rainy.Color, times int, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	times = max(1, min(10, times))
	l.color, l.times, l.start = c, times, now
	l.until = now.Add(time.Duration(times) * flashPeriod)
}

func (l *FlashLayer) Render(now time.Time, frame []rainy.Color) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !now.Before(l.until) {
		return false
	}
	// Каждая вспышка — быстрый подъём и спад.
	phase := math.Mod(float64(now.Sub(l.start))/float64(flashPeriod), 1)
	level := math.Sin(math.Pi * phase)
	for i := range frame {
		blend(&frame[i], l.color, level)
	}
	return true
}

type ReactiveConfig struct {
	Enabled  bool   `json:"enabled"`
	Style    string `json:"style"`    // fade | ripple | cross | heat
	Color    string `json:"color"`    // #rrggbb
	Rainbow  bool   `json:"rainbow"`  // каждое нажатие — своим цветом
	Duration int    `json:"duration"` // мс, сколько живёт след нажатия
}

func DefaultReactive() ReactiveConfig {
	return ReactiveConfig{Enabled: false, Style: "ripple", Color: "#7c8cff", Rainbow: false, Duration: 900}
}

func (c ReactiveConfig) Validate() (ReactiveConfig, error) {
	switch c.Style {
	case "fade", "ripple", "cross", "heat":
	default:
		return c, i18n.Errorf("неизвестный стиль реакции %q", c.Style)
	}
	if _, err := ParseColor(c.Color); err != nil {
		return c, err
	}
	c.Duration = max(200, min(4000, c.Duration))
	return c, nil
}

type press struct {
	pos int
	at  time.Time
	hue float64
}

type ReactiveLayer struct {
	mu      sync.Mutex
	cfg     ReactiveConfig
	presses []press
	heat    []float64
	heatAt  time.Time
	n       int
}

func NewReactive(cfg ReactiveConfig) *ReactiveLayer {
	return &ReactiveLayer{cfg: cfg, heat: make([]float64, rainy.Positions)}
}

func (l *ReactiveLayer) SetConfig(cfg ReactiveConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cfg = cfg
	if !cfg.Enabled {
		l.presses = nil
		clear(l.heat)
	}
}

func (l *ReactiveLayer) Press(pos int, now time.Time) {
	if pos < 0 || pos >= rainy.Positions {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.cfg.Enabled {
		return
	}
	l.n++
	// Золотое сечение разносит оттенки соседних нажатий.
	l.presses = append(l.presses, press{pos: pos, at: now, hue: math.Mod(float64(l.n)*0.618034, 1)})
	if len(l.presses) > 64 {
		l.presses = l.presses[len(l.presses)-64:]
	}
	if l.cfg.Style == "heat" {
		l.decayHeat(now)
		l.heat[pos] = min(1, l.heat[pos]+0.3)
	}
}

func (l *ReactiveLayer) decayHeat(now time.Time) {
	if !l.heatAt.IsZero() {
		k := math.Exp(-now.Sub(l.heatAt).Seconds() / (float64(l.cfg.Duration) / 1000 * 4))
		for i := range l.heat {
			l.heat[i] *= k
		}
	}
	l.heatAt = now
}

func (l *ReactiveLayer) colorOf(p press) rainy.Color {
	if l.cfg.Rainbow {
		return hsv(p.hue, 1, 1)
	}
	return mustColor(l.cfg.Color)
}

func (l *ReactiveLayer) Render(now time.Time, frame []rainy.Color) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.cfg.Enabled {
		return false
	}
	dur := time.Duration(l.cfg.Duration) * time.Millisecond
	alive := l.presses[:0]
	for _, p := range l.presses {
		if now.Sub(p.at) < dur {
			alive = append(alive, p)
		}
	}
	l.presses = alive

	if l.cfg.Style == "heat" {
		l.decayHeat(now)
		active := false
		for i, h := range l.heat {
			if h > 0.02 {
				// Холодное — синее, горячее — красное.
				blend(&frame[i], hsv(0.66*(1-h), 1, 1), 0.35+0.65*h)
				active = true
			}
		}
		return active
	}
	if len(l.presses) == 0 {
		return false
	}
	for _, p := range l.presses {
		age := float64(now.Sub(p.at)) / float64(dur) // 0…1
		fade := 1 - age
		c := l.colorOf(p)
		px, py := centers[p.pos][0], centers[p.pos][1]
		switch l.cfg.Style {
		case "fade":
			blend(&frame[p.pos], c, fade)
		case "cross":
			for i, ctr := range centers {
				dx, dy := math.Abs(ctr[0]-px), math.Abs(ctr[1]-py)
				var level float64
				switch {
				case dy < 0.3: // свой ряд
					level = math.Max(0, 1-dx/(1+age*8))
				case dx < 0.6: // своя колонка
					level = math.Max(0, 1-dy/(1+age*4))
				}
				if level > 0 {
					blend(&frame[i], c, level*fade)
				}
			}
		default: // ripple — расходящееся кольцо
			r := age * 7
			for i, ctr := range centers {
				d := math.Hypot(ctr[0]-px, ctr[1]-py)
				level := math.Exp(-(d-r)*(d-r)/0.5) * fade
				if d < 0.6 {
					level = math.Max(level, fade)
				}
				if level > 0.02 {
					blend(&frame[i], c, level)
				}
			}
		}
	}
	return true
}

type CapsConfig struct {
	Enabled bool   `json:"enabled"`
	Keys    string `json:"keys"` // caps | letters | all
	Color   string `json:"color"`
}

func DefaultCaps() CapsConfig { return CapsConfig{Enabled: false, Keys: "caps", Color: "#ffffff"} }

func (c CapsConfig) Validate() (CapsConfig, error) {
	if err := validKeys(c.Keys, "caps", "letters", "all"); err != nil {
		return c, err
	}
	_, err := ParseColor(c.Color)
	return c, err
}

type CapsLayer struct {
	mu  sync.Mutex
	cfg CapsConfig
	on  bool
}

func (l *CapsLayer) SetConfig(cfg CapsConfig) { l.mu.Lock(); l.cfg = cfg; l.mu.Unlock() }

func (l *CapsLayer) Set(on bool) { l.mu.Lock(); l.on = on; l.mu.Unlock() }

func (l *CapsLayer) Render(_ time.Time, frame []rainy.Color) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.cfg.Enabled || !l.on {
		return false
	}
	c := mustColor(l.cfg.Color)
	for _, p := range keysOf(l.cfg.Keys) {
		frame[p] = c
	}
	return true
}

type LangConfig struct {
	Enabled    bool   `json:"enabled"`
	Keys       string `json:"keys"` // esc | caps | space | frow | all
	EnColor    string `json:"enColor"`
	RuColor    string `json:"ruColor"`
	OtherColor string `json:"otherColor"`
	ShowFor    int    `json:"showFor"` // мс после смены раскладки; 0 — всегда
}

func DefaultLang() LangConfig {
	return LangConfig{Enabled: false, Keys: "esc", EnColor: "#2a6bff", RuColor: "#ff2a3c", OtherColor: "#2bd96b", ShowFor: 0}
}

func (c LangConfig) Validate() (LangConfig, error) {
	if err := validKeys(c.Keys, "esc", "caps", "space", "frow", "all"); err != nil {
		return c, err
	}
	for _, h := range []string{c.EnColor, c.RuColor, c.OtherColor} {
		if _, err := ParseColor(h); err != nil {
			return c, err
		}
	}
	c.ShowFor = max(0, min(10000, c.ShowFor))
	return c, nil
}

type LangLayer struct {
	mu        sync.Mutex
	cfg       LangConfig
	lang      string
	changedAt time.Time
}

func (l *LangLayer) SetConfig(cfg LangConfig) { l.mu.Lock(); l.cfg = cfg; l.mu.Unlock() }

func (l *LangLayer) Set(lang string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if lang != l.lang {
		l.lang, l.changedAt = lang, now
	}
}

func (l *LangLayer) Render(now time.Time, frame []rainy.Color) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.cfg.Enabled || l.lang == "" {
		return false
	}
	if l.cfg.ShowFor > 0 && now.Sub(l.changedAt) > time.Duration(l.cfg.ShowFor)*time.Millisecond {
		return false
	}
	hex := l.cfg.OtherColor
	switch {
	case strings.HasPrefix(l.lang, "en"):
		hex = l.cfg.EnColor
	case strings.HasPrefix(l.lang, "ru"):
		hex = l.cfg.RuColor
	}
	c := mustColor(hex)
	for _, p := range keysOf(l.cfg.Keys) {
		frame[p] = c
	}
	return true
}

type MeterConfig struct {
	Enabled bool   `json:"enabled"`
	Source  string `json:"source"` // cpu | volume
	Mode    string `json:"mode"`   // always | change
	Low     string `json:"low"`    // цвет начала шкалы
	High    string `json:"high"`   // цвет конца шкалы
}

func DefaultMeter() MeterConfig {
	return MeterConfig{Enabled: false, Source: "volume", Mode: "change", Low: "#2bd96b", High: "#ff2a3c"}
}

func (c MeterConfig) Validate() (MeterConfig, error) {
	if c.Source != "cpu" && c.Source != "volume" {
		return c, i18n.Errorf("неизвестный источник шкалы %q", c.Source)
	}
	if c.Mode != "always" && c.Mode != "change" {
		return c, i18n.Errorf("неизвестный режим шкалы %q", c.Mode)
	}
	for _, h := range []string{c.Low, c.High} {
		if _, err := ParseColor(h); err != nil {
			return c, err
		}
	}
	return c, nil
}

// meterShowFor — сколько видна шкала после изменения в режиме «при изменении».
const meterShowFor = 2 * time.Second

type MeterLayer struct {
	mu        sync.Mutex
	cfg       MeterConfig
	value     float64
	known     bool
	changedAt time.Time
}

func (l *MeterLayer) SetConfig(cfg MeterConfig) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if cfg.Source != l.cfg.Source {
		l.known = false // значение старого источника больше не годится
	}
	l.cfg = cfg
}

func (l *MeterLayer) Config() MeterConfig { l.mu.Lock(); defer l.mu.Unlock(); return l.cfg }

func (l *MeterLayer) Set(v float64, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	v = clamp01(v)
	if l.known && math.Abs(v-l.value) >= 0.01 {
		l.changedAt = now
	}
	l.value, l.known = v, true
}

func (l *MeterLayer) Render(now time.Time, frame []rainy.Color) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.cfg.Enabled || !l.known {
		return false
	}
	if l.cfg.Mode == "change" && now.Sub(l.changedAt) > meterShowFor {
		return false
	}
	low, high := mustColor(l.cfg.Low), mustColor(l.cfg.High)
	filled := l.value * float64(len(numberRow))
	for i, p := range numberRow {
		c := low
		blend(&c, high, float64(i)/float64(len(numberRow)-1))
		switch {
		case float64(i+1) <= filled:
			frame[p] = c
		case float64(i) < filled: // последняя клавиша — частично
			frame[p] = scale(c, filled-float64(i))
		default:
			frame[p] = scale(frame[p], 0.12)
		}
	}
	return true
}

type PomodoroConfig struct {
	Work       int    `json:"work"`  // минут работы
	Break      int    `json:"break"` // минут отдыха
	Keys       string `json:"keys"`  // frow | numrow
	WorkColor  string `json:"workColor"`
	BreakColor string `json:"breakColor"`
}

// DefaultPomodoro — классические 25/5.
func DefaultPomodoro() PomodoroConfig {
	return PomodoroConfig{Work: 25, Break: 5, Keys: "frow", WorkColor: "#ff2a3c", BreakColor: "#2bd96b"}
}

func (c PomodoroConfig) Validate() (PomodoroConfig, error) {
	if err := validKeys(c.Keys, "frow", "numrow"); err != nil {
		return c, err
	}
	for _, h := range []string{c.WorkColor, c.BreakColor} {
		if _, err := ParseColor(h); err != nil {
			return c, err
		}
	}
	c.Work = max(1, min(180, c.Work))
	c.Break = max(1, min(60, c.Break))
	return c, nil
}

type PomodoroState struct {
	Phase   string `json:"phase"` // idle | work | break
	Running bool   `json:"running"`
	Left    int    `json:"left"`  // секунд до конца фазы
	Total   int    `json:"total"` // длина фазы, секунд
}

// PomodoroLayer — таймер: прогресс фазы на ряду клавиш, вспышка на смене фаз.
type PomodoroLayer struct {
	mu      sync.Mutex
	cfg     PomodoroConfig
	phase   string
	end     time.Time     // конец фазы, если идёт
	left    time.Duration // остаток, если на паузе
	flash   *FlashLayer
	onPhase func(PomodoroState) // смена фазы — для уведомлений
}

func NewPomodoro(cfg PomodoroConfig, flash *FlashLayer, onPhase func(PomodoroState)) *PomodoroLayer {
	return &PomodoroLayer{cfg: cfg, phase: "idle", flash: flash, onPhase: onPhase}
}

// SetConfig меняет настройки (идущая фаза досчитывается по старым).
func (l *PomodoroLayer) SetConfig(cfg PomodoroConfig) { l.mu.Lock(); l.cfg = cfg; l.mu.Unlock() }

func (l *PomodoroLayer) phaseLen(phase string) time.Duration {
	if phase == "break" {
		return time.Duration(l.cfg.Break) * time.Minute
	}
	return time.Duration(l.cfg.Work) * time.Minute
}

func (l *PomodoroLayer) stateLocked(now time.Time) PomodoroState {
	st := PomodoroState{Phase: l.phase, Running: !l.end.IsZero()}
	if l.phase == "idle" {
		return st
	}
	left := l.left
	if st.Running {
		left = l.end.Sub(now)
	}
	st.Left = int(math.Ceil(max(0, left.Seconds())))
	st.Total = int(l.phaseLen(l.phase).Seconds())
	return st
}

func (l *PomodoroLayer) State(now time.Time) PomodoroState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stateLocked(now)
}

func (l *PomodoroLayer) Start(now time.Time) PomodoroState {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.phase == "idle":
		l.phase, l.end = "work", now.Add(l.phaseLen("work"))
	case l.end.IsZero():
		l.end = now.Add(l.left)
	}
	return l.stateLocked(now)
}

func (l *PomodoroLayer) Pause(now time.Time) PomodoroState {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.end.IsZero() {
		l.left, l.end = l.end.Sub(now), time.Time{}
	}
	return l.stateLocked(now)
}

func (l *PomodoroLayer) Stop(now time.Time) PomodoroState {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.phase, l.end, l.left = "idle", time.Time{}, 0
	return l.stateLocked(now)
}

func (l *PomodoroLayer) Skip(now time.Time) PomodoroState {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.phase != "idle" {
		l.nextLocked(now)
	}
	return l.stateLocked(now)
}

func (l *PomodoroLayer) nextLocked(now time.Time) {
	next, color := "break", l.cfg.BreakColor
	if l.phase == "break" {
		next, color = "work", l.cfg.WorkColor
	}
	l.phase, l.end, l.left = next, now.Add(l.phaseLen(next)), 0
	l.flash.Trigger(mustColor(color), 3, now)
	if l.onPhase != nil {
		st := l.stateLocked(now)
		go l.onPhase(st)
	}
}

func (l *PomodoroLayer) Render(now time.Time, frame []rainy.Color) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.phase == "idle" {
		return false
	}
	if !l.end.IsZero() && !now.Before(l.end) {
		l.nextLocked(now)
	}
	left := l.left
	if !l.end.IsZero() {
		left = l.end.Sub(now)
	}
	frac := clamp01(left.Seconds() / l.phaseLen(l.phase).Seconds()) // сколько осталось
	color := mustColor(l.cfg.WorkColor)
	if l.phase == "break" {
		color = mustColor(l.cfg.BreakColor)
	}
	keys := keysOf(l.cfg.Keys)
	lit := frac * float64(len(keys))
	paused := l.end.IsZero()
	for i, p := range keys {
		level := clamp01(lit - float64(i))
		if paused { // на паузе шкала мерцает
			level *= 0.45 + 0.35*math.Sin(float64(now.UnixMilli())/300)
		}
		frame[p] = scale(color, max(level, 0.06))
	}
	return true
}
