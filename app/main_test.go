package main

import "testing"

func TestParseFlash(t *testing.T) {
	cases := []struct {
		args  []string
		ok    bool
		color string
		times int
	}{
		{[]string{"rainy75"}, false, "#ffffff", 3},
		{[]string{"rainy75", "--flash"}, true, "#ffffff", 3},
		{[]string{"rainy75", "--flash", "#ff0000", "--times", "5"}, true, "#ff0000", 5},
		{[]string{"rainy75", "--times", "2", "--flash"}, true, "#ffffff", 2},
	}
	for _, c := range cases {
		f, ok := parseFlash(c.args)
		if ok != c.ok || f.color != c.color || f.times != c.times {
			t.Errorf("%v: %+v %v", c.args, f, ok)
		}
	}
}
