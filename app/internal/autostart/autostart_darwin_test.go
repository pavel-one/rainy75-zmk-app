package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if Enabled() {
		t.Fatal("в пустой папке автозапуск не может быть включён")
	}
	if err := Set(true); err != nil {
		t.Fatal(err)
	}
	if !Enabled() {
		t.Fatal("автозапуск не включился")
	}
	data, err := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", ID+".plist"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<string>"+HiddenFlag+"</string>") || !strings.Contains(string(data), "<true/>") {
		t.Fatalf("plist:\n%s", data)
	}
	if err := Set(false); err != nil || Enabled() {
		t.Fatalf("выключение: %v, включён=%v", err, Enabled())
	}
	if err := Set(false); err != nil {
		t.Fatalf("повторное выключение: %v", err)
	}
}

func TestPlistEscapesPath(t *testing.T) {
	p := string(plist(`/Apps/R&D <tools>/rainy75`))
	if !strings.Contains(p, "R&amp;D &lt;tools&gt;") {
		t.Fatalf("путь не экранирован:\n%s", p)
	}
}
