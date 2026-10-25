package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestTUIOSPreservesTerminalTransparencyAndSolidDock(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../.."))
	data, err := os.ReadFile(filepath.Join(root, "config/tuios/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Appearance struct {
			DockCompact    bool   `toml:"dock_compact"`
			Theme          string `toml:"theme"`
			Background     string `toml:"background"`
			PaneBackground string `toml:"pane_background"`
			DockBackground string `toml:"dock_background"`
			Sidebar        struct {
				Background string `toml:"background"`
			} `toml:"sidebar"`
		} `toml:"appearance"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Appearance.DockCompact {
		t.Error("TUIOS dock must not draw the unwanted horizontal separator")
	}
	if cfg.Appearance.Theme != "ayu-dots" {
		t.Fatalf("daily UI must keep Ayu, got %q", cfg.Appearance.Theme)
	}
	for name, value := range map[string]string{
		"canvas": cfg.Appearance.Background,
		"pane":   cfg.Appearance.PaneBackground,
	} {
		if value != "off" {
			t.Errorf("%s must leave default terminal cells unpainted so Rio/Ghostty transparency works: %q", name, value)
		}
	}
	for name, value := range map[string]string{
		"dock":    cfg.Appearance.DockBackground,
		"sidebar": cfg.Appearance.Sidebar.Background,
	} {
		if value != "theme" {
			t.Errorf("%s must keep legible Ayu chrome over the transparent terminal: %q", name, value)
		}
	}
}
