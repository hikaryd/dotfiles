package palette

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// A missing Rio target, swapped ANSI fields or destructive regeneration breaks
// the theme Rio actually consumes, not merely a source-code string.
func TestRioThemeSwitchPreservesTerminalConfig(t *testing.T) {
	root := fixture(t)
	config := filepath.Join(root, "config/rio/config.toml")
	if err := os.MkdirAll(filepath.Dir(config), 0755); err != nil {
		t.Fatal(err)
	}
	base := []byte("theme = 'ayu'\nline-height = 1.12\n[shell]\nprogram = '/bin/sh'\nargs = ['-c', 'exec \"$HOME/.local/bin/dots-nu\"']\n")
	mustWrite(t, config, base)
	for _, tc := range []struct {
		variant, background, foreground, selection, red, blue, brightBlue string
	}{
		{"dark", "#0d1017", "#bfbdb6", "#273747", "#ea6c73", "#53bdfa", "#59c2ff"},
		{"mirage", "#1f2430", "#cccac2", "#34455a", "#f28779", "#5ccfe6", "#73d0ff"},
		{"dark", "#0d1017", "#bfbdb6", "#273747", "#ea6c73", "#53bdfa", "#59c2ff"},
	} {
		var out, stderr bytes.Buffer
		if code := run(root, []string{"--tool", "rio", "--variant", tc.variant}, &out, &stderr); code != 0 {
			t.Fatalf("Rio %s generation failed (%d): %s", tc.variant, code, &stderr)
		}
		if !bytes.Equal(mustRead(t, config), base) {
			t.Fatal("palette regeneration overwrote terminal settings")
		}
		data := mustRead(t, filepath.Join(root, "config/rio/themes/ayu.toml"))
		var theme struct {
			Colors map[string]string `toml:"colors"`
		}
		if err := toml.Unmarshal(data, &theme); err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]string{
			"background": tc.background, "foreground": tc.foreground,
			"selection-background": tc.selection, "selection-foreground": tc.foreground,
			"red": tc.red, "blue": tc.blue, "light-blue": tc.brightBlue,
			"cursor": "#73d0ff",
		} {
			if got := theme.Colors[key]; got != want {
				t.Errorf("%s %s: got %q, want %q", tc.variant, key, got, want)
			}
		}
		before := snapshot(t, root)
		out.Reset()
		if code := run(root, []string{"--tool", "rio", "--variant", tc.variant}, &out, &stderr); code != 0 {
			t.Fatalf("repeat generation: %d %s", code, &stderr)
		}
		if !bytes.Equal(mustRead(t, filepath.Join(root, "config/rio/themes/ayu.toml")), data) || len(snapshot(t, root)) != len(before) {
			t.Fatal("repeat generation changed theme or created unexpected files")
		}
	}
}
