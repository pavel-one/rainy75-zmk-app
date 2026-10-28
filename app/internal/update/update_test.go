package update

import "testing"

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.2.0", "0.1.0", true},
		{"v1.0.0", "0.9.9", true},
		{"0.1.10", "0.1.9", true},
		{"0.1.0", "0.1.0", false},
		{"0.1.0", "0.2.0", false},
		{"0.2.0", "0.2.0-dev.5", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestSafePath(t *testing.T) {
	if _, err := safePath("/tmp/x", "../evil"); err == nil {
		t.Fatal("путь за пределы архива пропущен")
	}
	if _, err := safePath("/tmp/x", "Rainy75.app/Contents/Info.plist"); err != nil {
		t.Fatal(err)
	}
}
