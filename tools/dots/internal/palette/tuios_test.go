package palette

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTUIOSThemeFollowsPaletteAndPreservesKeys(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "config/tuios/config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, path, []byte("[keybindings]\nleader_key = 'ctrl+a'\n"))
	for _, tc := range []struct{ variant, bg, fg, blue string }{
		{"dark", "#0d1017", "#bfbdb6", "#53bdfa"},
		{"mirage", "#1f2430", "#cccac2", "#5ccfe6"},
	} {
		var out, stderr bytes.Buffer
		if code := run(root, []string{"--tool", "tuios", "--variant", tc.variant}, &out, &stderr); code != 0 {
			t.Fatalf("generation %s: %d %s", tc.variant, code, &stderr)
		}
		var theme map[string]any
		if err := json.Unmarshal(mustRead(t, filepath.Join(root, "config/tuios/themes/ayu-dots.json")), &theme); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]string{"id": "ayu-dots", "bg": tc.bg, "fg": tc.fg, "blue": tc.blue, "cursor": "#73d0ff"} {
			if theme[key] != want {
				t.Errorf("%s %s: %v, want %s", tc.variant, key, theme[key], want)
			}
		}
		chrome := theme["chrome"].(map[string]any)
		if chrome["accent"] != "#73d0ff" || chrome["surface"] != tc.bg {
			t.Fatalf("chrome not Ayu: %v", chrome)
		}
		if string(mustRead(t, path)) != "[keybindings]\nleader_key = 'ctrl+a'\n" {
			t.Fatal("theme overwrote keybindings")
		}
	}
}
