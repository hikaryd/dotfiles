package palette

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func assertTerminalPalette(t *testing.T, text string, p palette, ghostty bool) {
	t.Helper()
	expected := map[string]string{
		"background":           strings.TrimPrefix(p["editor"]["bg"], "#"),
		"foreground":           strings.TrimPrefix(p["editor"]["fg"], "#"),
		"selection-background": strings.TrimPrefix(p["ui"]["selection"], "#"),
		"selection-foreground": strings.TrimPrefix(p["editor"]["fg"], "#"),
		"cursor-color":         strings.TrimPrefix(p["common"]["accent"], "#"),
	}
	if ghostty {
		expected["unfocused-split-fill"] = strings.TrimPrefix(p["editor"]["bg"], "#")
	}
	keys := []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white", "bright_black", "bright_red", "bright_green", "bright_yellow", "bright_blue", "bright_magenta", "bright_cyan", "bright_white"}
	for i, key := range keys {
		expected["palette:"+strconv.Itoa(i)] = p["terminal"][key]
	}
	found := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key == "theme" {
			t.Fatalf("named theme can leak colors: %s", line)
		}
		if key == "palette" {
			index, color, ok := strings.Cut(value, "=")
			if !ok {
				t.Fatalf("invalid palette entry %q", line)
			}
			key, value = "palette:"+strings.TrimSpace(index), strings.TrimSpace(color)
		}
		if _, ok := expected[key]; !ok {
			if strings.HasPrefix(key, "palette:") {
				t.Fatalf("unexpected ANSI entry %q", line)
			}
			continue
		}
		if _, duplicate := found[key]; duplicate {
			t.Fatalf("duplicate %s", key)
		}
		found[key] = value
	}
	if !reflect.DeepEqual(found, expected) {
		t.Fatalf("terminal colors mismatch\ngot: %v\nwant: %v", found, expected)
	}
}

func TestFixtureTerminalPalettes(t *testing.T) {
	cases := []struct{ name, file, variant string }{
		{"dark", "testdata/input/palette/ayu.toml", "dark"},
		{"mirage", "testdata/input/palette/ayu.toml", "mirage"},
		{"custom", "testdata/custom.toml", "dark"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			data := mustRead(t, tc.file)
			mustWrite(t, filepath.Join(root, "palette/ayu.toml"), data)
			var doc struct {
				Dark   palette `toml:"dark"`
				Mirage palette `toml:"mirage"`
			}
			if err := toml.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			p := doc.Dark
			if tc.variant == "mirage" {
				p = doc.Mirage
			}
			for _, tool := range []string{"ghostty", "rex"} {
				var out, stderr bytes.Buffer
				if code := run(root, []string{"--variant", tc.variant, "--tool", tool}, &out, &stderr); code != 0 {
					t.Fatalf("%s: %s", tool, &stderr)
				}
				path := "config/ghostty/config"
				if tool == "rex" {
					path = "config/rex/ayu-dark"
				}
				t.Run(tool, func(t *testing.T) {
					assertTerminalPalette(t, string(mustRead(t, filepath.Join(root, path))), p, tool == "ghostty")
				})
			}
		})
	}
}

func TestRepositoryGeneratedFilesCurrent(t *testing.T) {
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	root := fixture(t)
	inputs := []string{"palette/ayu.toml", "config/ghostty/config", "config/starship/starship.toml", "config/tmux/tmux.conf", "config/lazygit/config.yml"}
	for _, path := range inputs {
		mustWrite(t, filepath.Join(root, path), mustRead(t, filepath.Join(repo, path)))
	}
	var out, stderr bytes.Buffer
	if code := run(root, nil, &out, &stderr); code != 0 {
		t.Fatalf("generation: %s", &stderr)
	}
	outputs := []string{
		"config/ghostty/config", "config/rex/ayu-dark", "config/rio/themes/ayu.toml", "config/starship/starship.toml", "config/nushell/ayu.nu", "config/sketchybar/colors.sh", "config/tmux/tmux.conf", "config/fresh/themes/ayu-dark.json", "config/lazygit/config.yml", "config/thorium/ayu-dark/manifest.json", "config/yazi/flavors/ayu-dark.yazi/flavor.toml", "config/yazi/flavors/ayu-dark.yazi/tmtheme.xml", "config/yazi/flavors/ayu-dark.yazi/LICENSE", "config/yazi/flavors/ayu-dark.yazi/LICENSE-tmtheme", "config/yazi/theme.toml", "config/yazi/init.lua", "config/jcode/theme.toml",
	}
	for _, path := range outputs {
		got := mustRead(t, filepath.Join(root, path))
		want, err := os.ReadFile(filepath.Join(repo, path))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from fresh generation", path)
		}
	}
	if t.Failed() {
		t.Log(fmt.Sprintf("generator output: %s", &out))
	}
}
