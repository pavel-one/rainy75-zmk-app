package layout

import "testing"

func TestRainy75Layout(t *testing.T) {
	l := Rainy75
	if len(l.Keys) != 83 {
		t.Fatalf("клавиш %d, нужно 83 позиции keymap", len(l.Keys))
	}
	if l.Width < 16 || l.Width > 16.5 || l.Height != 6.1 {
		t.Fatalf("размер %.2f×%.2f", l.Width, l.Height)
	}
	codes := map[string]int{}
	for i, k := range l.Keys {
		if k.Pos != i {
			t.Fatalf("позиция %d записана как %d", i, k.Pos)
		}
		if k.Code != "" {
			if prev, dup := codes[k.Code]; dup {
				t.Fatalf("код %s у позиций %d и %d", k.Code, prev, i)
			}
			codes[k.Code] = i
		}
	}
	// Позиции из rgb_mgmt / rainy75_rgb.py: пробел — 76, стрелка вправо — последняя.
	if l.Keys[76].Code != "Space" || l.Keys[82].Code != "ArrowRight" || l.Keys[14].Code != "Home" {
		t.Fatalf("позиции: %q %q %q", l.Keys[76].Code, l.Keys[82].Code, l.Keys[14].Code)
	}
	// В режиме Mac вторая клавиша нижнего ряда — ⌥, третья — ⌘.
	if l.Keys[74].MacCode != "AltLeft" || l.Keys[75].MacCode != "MetaLeft" || l.Keys[74].Code != "MetaLeft" {
		t.Fatalf("модификаторы: %+v %+v", l.Keys[74], l.Keys[75])
	}
}
