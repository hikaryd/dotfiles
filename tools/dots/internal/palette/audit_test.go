package palette

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStarshipPreservesTrailingTables(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "config/starship/starship.toml")
	tail := "\n[custom.audit]\ncommand = \"printf audit\"\nwhen = true\n"
	mustWrite(t, path, append(mustRead(t, path), []byte(tail)...))
	for _, variant := range []string{"dark", "mirage", "mirage"} {
		var out, diagnostic bytes.Buffer
		if code := run(root, []string{"--tool", "starship", "--variant", variant}, &out, &diagnostic); code != 0 {
			t.Fatal(code, diagnostic.String())
		}
		if !strings.HasSuffix(string(mustRead(t, path)), tail) {
			t.Fatal("unrelated trailing TOML table removed")
		}
	}
}

func TestLazygitValidThemeRepresentations(t *testing.T) {
	for _, input := range []string{
		"gui:\n  theme: # personal colors\n    activeBorderColor: [red]\n  showIcons: true\n",
		"gui:\n  theme: {}\n  showIcons: true\n",
		"gui:\n    theme:\n        activeBorderColor: [red]\n    showIcons: true\n",
		"gui: {theme: {}, showIcons: true}\n",
	} {
		t.Run(input, func(t *testing.T) {
			root := fixture(t)
			path := filepath.Join(root, "config/lazygit/config.yml")
			mustWrite(t, path, []byte(input))
			for pass := 0; pass < 2; pass++ {
				var out, diagnostic bytes.Buffer
				if code := run(root, []string{"--tool", "lazygit"}, &out, &diagnostic); code != 0 {
					t.Fatal(code, diagnostic.String())
				}
				var doc struct {
					GUI struct {
						Theme map[string]any `yaml:"theme"`
						Icons bool           `yaml:"showIcons"`
					} `yaml:"gui"`
				}
				if err := yaml.Unmarshal(mustRead(t, path), &doc); err != nil {
					t.Fatalf("invalid generated YAML: %v", err)
				}
				if !doc.GUI.Icons || len(doc.GUI.Theme) < 2 {
					t.Fatalf("lost neighbor or failed theme update: %+v", doc)
				}
			}
		})
	}
}
