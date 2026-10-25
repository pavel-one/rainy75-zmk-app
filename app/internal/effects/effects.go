// Package effects рисует анимации подсветки на компьютере: кадр из 83 цветов
// считается здесь и отправляется в клавиатуру целиком.
package effects

import (
	"math"

	"rainy75/internal/layout"
	"rainy75/internal/rainy"
)

type Effect struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Colors int    `json:"colors"` // 0 — своя палитра, 1 или 2 — цвета пользователя
}

type px struct {
	i    int     // позиция keymap
	x, y float64 // центр клавиши, 0…1
	row  int     // ряд схемы, 0…5
}

type ctx struct {
	phase  float64 // время с учётом скорости
	bright float64
	color  rainy.Color
	color2 rainy.Color
}

type render func(c ctx, p px) rainy.Color

type entry struct {
	Effect
	draw render
}

// aspect — во сколько раз клавиатура шире, чем выше: для кругов и диагоналей.
var aspect = layout.Rainy75.Width / layout.Rainy75.Height

var table = []entry{
	// Свои цвета.
	{Effect{"rainbow", "Радуга", 0}, func(c ctx, p px) rainy.Color {
		return HSV(p.x*0.55+p.y*0.18-c.phase*0.08, 1, c.bright)
	}},
	{Effect{"rainbow-breathe", "Дыхание радуги", 0}, func(c ctx, p px) rainy.Color {
		return HSV(c.phase*0.05, 1, c.bright*(0.15+0.85*wave(c.phase*1.6)))
	}},
	{Effect{"plasma", "Плазма", 0}, func(c ctx, p px) rainy.Color {
		x, y := p.x*aspect, p.y
		v := math.Sin(x*3.2+c.phase) + math.Sin(y*8-c.phase*1.3) +
			math.Sin((x+y*2)*2.2+c.phase*0.7) + math.Sin(math.Hypot(x-aspect/2, y-0.5)*6-c.phase*1.6)
		return HSV(v/8+c.phase*0.04, 1, c.bright)
	}},
	{Effect{"aurora", "Северное сияние", 0}, func(c ctx, p px) rainy.Color {
		hue := 0.36 + 0.22*math.Sin(p.x*3+c.phase*0.35)
		band := wave(p.x*5 - c.phase*0.8 + math.Sin(p.y*3+c.phase)*1.5)
		return HSV(hue, 0.9, c.bright*(0.08+0.92*band*(0.45+0.55*(1-p.y))))
	}},
	{Effect{"ocean", "Океан", 0}, func(c ctx, p px) rainy.Color {
		v := wave(p.x*6+p.y*2-c.phase*1.4) * wave(p.x*2.5-p.y*3+c.phase*0.9)
		return HSV(0.52+0.08*v, 0.95-0.5*v*v, c.bright*(0.2+0.8*v))
	}},
	{Effect{"sunset", "Закат", 0}, func(c ctx, p px) rainy.Color {
		h := 0.98 + 0.12*(1-p.y) + 0.03*math.Sin(p.x*4+c.phase*0.6)
		return HSV(h, 0.95, c.bright*(0.55+0.45*wave(p.x*3-c.phase*0.5)))
	}},
	{Effect{"fire", "Огонь", 0}, func(c ctx, p px) rainy.Color {
		heat := clamp01((0.35 + 0.55*p.y + 0.18*math.Sin(p.x*22+c.phase*4+float64(p.i))) * c.bright)
		return HSV(0.02+0.10*heat, 1, heat)
	}},
	{Effect{"matrix", "Матрица", 0}, func(c ctx, p px) rainy.Color {
		col := math.Floor(p.x * 16)
		head := frac(c.phase*0.45*(0.6+hash(col)) + hash(col+40))
		d := head*1.4 - 0.2 - p.y // голова идёт сверху вниз
		switch {
		case d >= 0 && d < 0.06:
			return scale(rainy.Color{R: 190, G: 255, B: 190}, c.bright)
		case d >= 0 && d < 0.6:
			return scale(rainy.Color{R: 0, G: 255, B: 70}, c.bright*math.Pow(1-d/0.6, 2))
		}
		return scale(rainy.Color{G: 255}, c.bright*0.02)
	}},
	{Effect{"police", "Мигалка", 0}, func(c ctx, p px) rainy.Color {
		step := int(c.phase*6) % 8 // две вспышки слева, две справа
		left := p.x < 0.5
		on := (left && (step == 0 || step == 2)) || (!left && (step == 4 || step == 6))
		if !on {
			return rainy.Color{}
		}
		if left {
			return scale(rainy.Color{R: 255}, c.bright)
		}
		return scale(rainy.Color{B: 255, G: 40}, c.bright)
	}},

	// Своим цветом.
	{Effect{"wave", "Волна", 2}, func(c ctx, p px) rainy.Color {
		return scale(mix(c.color2, c.color, wave(p.x*9+p.y*5.5-c.phase*2.8)), c.bright)
	}},
	{Effect{"pulse", "Дыхание", 2}, func(c ctx, p px) rainy.Color {
		// Каждый вдох — другим цветом: нечётные — вторым.
		col := c.color
		if int(math.Floor((c.phase*2.2+math.Pi/2)/(2*math.Pi)))%2 != 0 {
			col = c.color2
		}
		return scale(col, (0.16+0.84*wave(c.phase*2.2))*c.bright)
	}},
	{Effect{"heartbeat", "Пульс", 1}, func(c ctx, p px) rainy.Color {
		t := frac(c.phase * 0.7)
		beat := gauss(t, 0.08, 0.035) + 0.65*gauss(t, 0.26, 0.04)
		fall := 1 - 0.35*math.Hypot((p.x-0.5)*aspect, p.y-0.5)/aspect
		return scale(c.color, (0.06+0.94*clamp01(beat)*fall)*c.bright)
	}},
	{Effect{"ripple", "Круги", 2}, func(c ctx, p px) rainy.Color {
		level := 0.04
		for k := range 3 { // три кольца из разных точек, каждое живёт 2.4 условных секунды
			slot := math.Floor(c.phase/0.8) - float64(k)
			age := c.phase - slot*0.8
			cx, cy := hash(slot)*aspect, 0.2+0.6*hash(slot+7)
			d := math.Hypot(p.x*aspect-cx, p.y-cy)
			ring := gauss(d, age*1.4, 0.14) * (1 - age/2.4)
			level = max(level, ring)
		}
		return scale(mix(c.color2, c.color, clamp01(level*1.6)), clamp01(level+0.06)*c.bright)
	}},
	{Effect{"comet", "Кометы", 2}, func(c ctx, p px) rainy.Color {
		r := float64(p.row)
		head := frac(c.phase*0.32*(0.8+0.5*hash(r)) + hash(r+9))
		d := head*1.3 - p.x // хвост тянется влево
		switch {
		case d >= 0 && d < 0.035:
			return mix(scale(c.color, c.bright), rainy.Color{R: 255, G: 255, B: 255}, 0.6*c.bright)
		case d >= 0 && d < 0.35:
			return scale(mix(c.color, c.color2, d/0.35), c.bright*math.Pow(1-d/0.35, 2))
		}
		return scale(c.color2, c.bright*0.03)
	}},
	{Effect{"scanner", "Сканер", 1}, func(c ctx, p px) rainy.Color {
		center := wave(c.phase * 1.6)
		return scale(c.color, max(0.03, 1-math.Abs(p.x-center)*7)*c.bright)
	}},
	{Effect{"candy", "Леденец", 2}, func(c ctx, p px) rainy.Color {
		v := frac((p.x*aspect+p.y)*0.55 - c.phase*0.25)
		if v < 0.5 {
			return scale(c.color, c.bright)
		}
		return scale(c.color2, c.bright)
	}},
	{Effect{"gradient", "Перелив", 2}, func(c ctx, p px) rainy.Color {
		return scale(mix(c.color, c.color2, wave(p.x*4-c.phase*0.9)), c.bright)
	}},
	{Effect{"stars", "Звёзды", 2}, func(c ctx, p px) rainy.Color {
		f := float64(p.i)
		tw := math.Pow(max(0, math.Sin(c.phase*(0.8+hash(f))*1.6+hash(f+3)*6.28)), 10)
		return scale(mix(c.color, c.color2, tw), (0.05+0.95*tw)*c.bright)
	}},
	{Effect{"sparkle", "Искры", 1}, func(c ctx, p px) rainy.Color {
		// Каждые 1/8 условной секунды загорается новый случайный набор клавиш.
		tick := math.Floor(c.phase * 8)
		level := 0.035
		if hash(float64(p.i)*1.37+tick*91.7) < 0.055 {
			level = 1
		}
		return scale(c.color, level*c.bright)
	}},
	{Effect{"rain", "Дождь", 2}, func(c ctx, p px) rainy.Color {
		streak := math.Sin(p.x*37 + p.y*11 - c.phase*5 + float64(p.i)*0.7)
		if streak > 0.83 {
			return scale(c.color, c.bright)
		}
		return scale(c.color2, max(0.02, (streak+1)*0.08)*c.bright)
	}},
	{Effect{"spiral", "Спираль", 2}, func(c ctx, p px) rainy.Color {
		dx, dy := p.x-0.5, p.y-0.5
		angle := math.Atan2(dy, dx) / (2 * math.Pi)
		level := wave((angle + math.Hypot(dx, dy)*1.7 - c.phase*0.12) * 2 * math.Pi)
		return scale(mix(c.color2, c.color, level), (0.25+level*0.75)*c.bright)
	}},
}

var List = func() []Effect {
	out := make([]Effect, len(table))
	for i, e := range table {
		out[i] = e.Effect
	}
	return out
}()

func find(key string) *entry {
	for i := range table {
		if table[i].Key == key {
			return &table[i]
		}
	}
	return nil
}

func Valid(key string) bool { return find(key) != nil }

type Params struct {
	Kind       string
	Color      rainy.Color
	Color2     rainy.Color // второй цвет двухцветных эффектов
	HueShift   float64     // сдвиг оттенка палитры, градусы
	Saturation float64     // насыщенность палитры, 0…1 (0 — как задумано)
	Speed      float64     // 0.25…3
	Brightness float64     // 0…1
}

// pixels — клавиши с центрами, нормированными в 0…1 (y=0 — верхний ряд).
var pixels = func() []px {
	l := layout.Rainy75
	out := make([]px, len(l.Keys))
	for i, k := range l.Keys {
		out[i] = px{i: i, x: (k.X + k.W/2) / l.Width, y: (k.Y + k.H/2) / l.Height, row: int(k.Y + 0.5)}
	}
	return out
}()

func Frame(p Params, t float64) []rainy.Color {
	e := find(p.Kind)
	if e == nil {
		e = &table[0]
	}
	c := ctx{phase: t * max(0.1, p.Speed), bright: clamp01(p.Brightness), color: p.Color, color2: p.Color2}
	// Палитру своих эффектов можно повернуть по кругу оттенков и приглушить.
	shift, sat := p.HueShift/360, p.Saturation
	tune := e.Colors == 0 && (shift != 0 || (sat > 0 && sat < 1))
	frame := make([]rainy.Color, len(pixels))
	for i, px := range pixels {
		col := e.draw(c, px)
		if tune {
			h, s, v := toHSV(col)
			if sat > 0 {
				s *= sat
			}
			col = HSV(h+shift, s, v)
		}
		frame[i] = col
	}
	return frame
}

func clamp01(v float64) float64 { return max(0, min(1, v)) }

func frac(v float64) float64 { return v - math.Floor(v) }

func wave(v float64) float64 { return (math.Sin(v) + 1) / 2 }

func gauss(v, mu, w float64) float64 { return math.Exp(-(v - mu) * (v - mu) / (2 * w * w)) }

// hash — псевдослучайное число 0…1, одинаковое для одного аргумента.
func hash(v float64) float64 { return frac(math.Sin(v*12.9898+78.233) * 43758.5453) }

func scale(c rainy.Color, level float64) rainy.Color {
	level = clamp01(level)
	return rainy.Color{R: uint8(float64(c.R) * level), G: uint8(float64(c.G) * level), B: uint8(float64(c.B) * level)}
}

func mix(a, b rainy.Color, t float64) rainy.Color {
	t = clamp01(t)
	m := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t) }
	return rainy.Color{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B)}
}

// HSV переводит оттенок (0…1), насыщенность и яркость в RGB.
func HSV(h, s, v float64) rainy.Color {
	h = frac(h)
	s, v = clamp01(s), clamp01(v)
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
	return rainy.Color{R: uint8(math.Round(r * 255)), G: uint8(math.Round(g * 255)), B: uint8(math.Round(b * 255))}
}

// toHSV — обратное преобразование: оттенок, насыщенность, яркость в 0…1.
func toHSV(c rainy.Color) (h, s, v float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx, mn := max(r, g, b), min(r, g, b)
	v = mx
	if mx == 0 {
		return 0, 0, 0
	}
	s = (mx - mn) / mx
	d := mx - mn
	switch {
	case d == 0:
		h = 0
	case mx == r:
		h = (g - b) / d / 6
	case mx == g:
		h = ((b-r)/d + 2) / 6
	default:
		h = ((r-g)/d + 4) / 6
	}
	return frac(h), s, v
}
