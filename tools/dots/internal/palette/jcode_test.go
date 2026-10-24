package palette

import (
	"bytes"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestJcodeTheme(t *testing.T) {
	for _, variant := range []string{"dark", "mirage", "custom"} {
		t.Run(variant, func(t *testing.T) {
			root := fixture(t)
			selected := variant
			if variant == "custom" {
				selected = "dark"
				mustWrite(t, filepath.Join(root, "palette/ayu.toml"), mustRead(t, "testdata/custom.toml"))
			}
			before := snapshot(t, root)
			var out, stderr bytes.Buffer
			if code := run(root, []string{"--tool", "jcode", "--variant", selected}, &out, &stderr); code != 0 {
				t.Fatalf("exit %d: %s", code, &stderr)
			}
			path := "config/jcode/theme.toml"
			data := mustRead(t, filepath.Join(root, path))
			want := mustRead(t, filepath.Join("testdata/golden", variant, path))
			if !bytes.Equal(data, want) {
				t.Fatalf("theme differs from shared palette golden\ngot: %s\nwant: %s", data, want)
			}
			before[path] = string(data)
			if !reflect.DeepEqual(before, snapshot(t, root)) {
				t.Fatal("jcode-only generation changed another application")
			}
			var theme struct {
				Display struct {
					Theme  string
					Colors map[string]string
				}
			}
			if err := toml.Unmarshal(data, &theme); err != nil {
				t.Fatal(err)
			}
			if theme.Display.Theme != "dark" || len(theme.Display.Colors) != 22 {
				t.Fatal("expected native dark appearance and 22 role colors")
			}
			for role, color := range theme.Display.Colors {
				if !regexp.MustCompile(`^#[0-9a-f]{6}$`).MatchString(color) {
					t.Fatalf("invalid native color %s=%s", role, color)
				}
			}
		})
	}
}
