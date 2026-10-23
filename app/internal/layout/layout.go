// Package layout — схема Rainy75 для открытой прошивки: 83 позиции keymap
// (ISO, построчно) с геометрией из rainy75.dts прошивки rainy75-studio.
package layout

// Key — клавиша. Координаты и размеры в юнитах (1u — обычная клавиша).
// Code — KeyboardEvent.code, который приходит в режиме Windows; MacCode —
// в режиме macOS (если отличается). Пустой код — клавиша не видна ОС (Fn).
type Key struct {
	Pos      int     `json:"pos"`
	Label    string  `json:"label"`
	MacLabel string  `json:"macLabel"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	W        float64 `json:"w"`
	H        float64 `json:"h"`
	Code     string  `json:"code"`
	MacCode  string  `json:"macCode"`
}

type Layout struct {
	Keys   []Key   `json:"keys"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// k описывает клавишу в сотых долях юнита, как в DTS.
type k struct {
	label, code string
	w, h, x, y  int
}

var rows = [][]k{
	{
		{"Esc", "Escape", 100, 100, 0, 0}, {"F1", "F1", 100, 100, 130, 0}, {"F2", "F2", 100, 100, 230, 0},
		{"F3", "F3", 100, 100, 330, 0}, {"F4", "F4", 100, 100, 430, 0}, {"F5", "F5", 100, 100, 560, 0},
		{"F6", "F6", 100, 100, 660, 0}, {"F7", "F7", 100, 100, 760, 0}, {"F8", "F8", 100, 100, 860, 0},
		{"F9", "F9", 100, 100, 980, 0}, {"F10", "F10", 100, 100, 1080, 0}, {"F11", "F11", 100, 100, 1180, 0},
		{"F12", "F12", 100, 100, 1280, 0}, {"Del", "Delete", 100, 100, 1425, 0}, {"Home", "Home", 100, 100, 1525, 0},
	},
	{
		{"`", "Backquote", 100, 100, 0, 110}, {"1", "Digit1", 100, 100, 100, 110}, {"2", "Digit2", 100, 100, 200, 110},
		{"3", "Digit3", 100, 100, 300, 110}, {"4", "Digit4", 100, 100, 400, 110}, {"5", "Digit5", 100, 100, 500, 110},
		{"6", "Digit6", 100, 100, 600, 110}, {"7", "Digit7", 100, 100, 700, 110}, {"8", "Digit8", 100, 100, 800, 110},
		{"9", "Digit9", 100, 100, 900, 110}, {"0", "Digit0", 100, 100, 1000, 110}, {"-", "Minus", 100, 100, 1100, 110},
		{"=", "Equal", 100, 100, 1200, 110}, {"⌫", "Backspace", 225, 100, 1300, 110}, {"End", "End", 100, 100, 1525, 110},
	},
	{
		{"Tab", "Tab", 165, 100, 0, 210}, {"Q", "KeyQ", 100, 100, 165, 210}, {"W", "KeyW", 100, 100, 265, 210},
		{"E", "KeyE", 100, 100, 365, 210}, {"R", "KeyR", 100, 100, 465, 210}, {"T", "KeyT", 100, 100, 565, 210},
		{"Y", "KeyY", 100, 100, 665, 210}, {"U", "KeyU", 100, 100, 765, 210}, {"I", "KeyI", 100, 100, 865, 210},
		{"O", "KeyO", 100, 100, 965, 210}, {"P", "KeyP", 100, 100, 1065, 210}, {"[", "BracketLeft", 100, 100, 1165, 210},
		{"]", "BracketRight", 100, 100, 1265, 210}, {"Enter", "Enter", 125, 200, 1385, 210},
	},
	{
		{"Caps", "CapsLock", 185, 100, 0, 310}, {"A", "KeyA", 100, 100, 185, 310}, {"S", "KeyS", 100, 100, 285, 310},
		{"D", "KeyD", 100, 100, 385, 310}, {"F", "KeyF", 100, 100, 485, 310}, {"G", "KeyG", 100, 100, 585, 310},
		{"H", "KeyH", 100, 100, 685, 310}, {"J", "KeyJ", 100, 100, 785, 310}, {"K", "KeyK", 100, 100, 885, 310},
		{"L", "KeyL", 100, 100, 985, 310}, {";", "Semicolon", 100, 100, 1085, 310}, {"'", "Quote", 100, 100, 1185, 310},
		{"#", "Backslash", 100, 100, 1285, 310}, {"PgUp", "PageUp", 100, 100, 1525, 210}, {"PgDn", "PageDown", 100, 100, 1525, 310},
	},
	{
		{"Shift", "ShiftLeft", 125, 100, 0, 410}, {"<>", "IntlBackslash", 100, 100, 125, 410}, {"Z", "KeyZ", 100, 100, 225, 410},
		{"X", "KeyX", 100, 100, 325, 410}, {"C", "KeyC", 100, 100, 425, 410}, {"V", "KeyV", 100, 100, 525, 410},
		{"B", "KeyB", 100, 100, 625, 410}, {"N", "KeyN", 100, 100, 725, 410}, {"M", "KeyM", 100, 100, 825, 410},
		{",", "Comma", 100, 100, 925, 410}, {".", "Period", 100, 100, 1025, 410}, {"/", "Slash", 100, 100, 1125, 410},
		{"Shift", "ShiftRight", 200, 100, 1225, 410}, {"↑", "ArrowUp", 100, 100, 1425, 410},
	},
	{
		{"Ctrl", "ControlLeft", 130, 100, 0, 510}, {"Win", "MetaLeft", 130, 100, 130, 510}, {"Alt", "AltLeft", 130, 100, 260, 510},
		{"", "Space", 620, 100, 390, 510}, {"Alt", "AltRight", 100, 100, 1010, 510}, {"Fn", "", 100, 100, 1110, 510},
		{"Ctrl", "ControlRight", 100, 100, 1210, 510}, {"←", "ArrowLeft", 100, 100, 1325, 510},
		{"↓", "ArrowDown", 100, 100, 1425, 510}, {"→", "ArrowRight", 100, 100, 1525, 510},
	},
}

// Отличия режима macOS (базовый слой прошивки): ⌥ и ⌘ вместо Win и Alt,
// F-ряд — функции Mac, которые браузер не видит как F-клавиши.
var macOverrides = map[int]struct{ label, code string }{
	1: {"☀−", ""}, 2: {"☀+", ""}, 3: {"⊞", ""}, 4: {"▦", ""},
	7: {"⏮", ""}, 8: {"⏯", ""}, 9: {"⏭", ""}, 10: {"🔇", ""}, 11: {"🔉", ""}, 12: {"🔊", ""},
	74: {"⌥", "AltLeft"}, 75: {"⌘", "MetaLeft"}, 77: {"⌘", "MetaRight"},
}

var Rainy75 = build()

func build() Layout {
	var l Layout
	for _, row := range rows {
		for _, r := range row {
			key := Key{
				Pos: len(l.Keys), Label: r.label, MacLabel: r.label, Code: r.code, MacCode: r.code,
				X: float64(r.x) / 100, Y: float64(r.y) / 100, W: float64(r.w) / 100, H: float64(r.h) / 100,
			}
			if o, ok := macOverrides[key.Pos]; ok {
				key.MacLabel, key.MacCode = o.label, o.code
			}
			l.Keys = append(l.Keys, key)
			l.Width = max(l.Width, key.X+key.W)
			l.Height = max(l.Height, key.Y+key.H)
		}
	}
	return l
}
