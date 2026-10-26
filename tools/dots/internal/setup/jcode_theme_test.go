package setup

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestJcodeAppliesOnlyOwnedThemeSettings(t *testing.T) {
	home, root := jcodeFixture(t)
	put(t, filepath.Join(root, "config/jcode/theme.toml"), "[display]\ntheme = 'dark'\n[display.colors]\naccent = '#73d0ff'\nai = '#aad94c'\n")
	path := filepath.Join(home, ".jcode/config.toml")
	put(t, path, "[provider]\ndefault_model = 'personal-model'\n[display]\ntheme = 'light'\nshow_token_usage = false\n[display.colors]\naccent = '#000000'\n[hooks]\nsession_end = 'personal-hook'\n")
	if err := jcodeApply(root); err != nil {
		t.Fatal(err)
	}
	var current map[string]any
	if err := toml.Unmarshal([]byte(get(t, path)), &current); err != nil {
		t.Fatal(err)
	}
	display := current["display"].(map[string]any)
	want := map[string]any{"accent": "#73d0ff", "ai": "#aad94c"}
	if display["theme"] != "dark" || !reflect.DeepEqual(display["colors"], want) {
		t.Fatalf("shared theme was not applied: %v", display)
	}
	if display["show_token_usage"] != false || current["provider"].(map[string]any)["default_model"] != "gpt-6.1-terra" || current["hooks"].(map[string]any)["session_end"] != "personal-hook" {
		t.Fatal("theme update overwrote personal settings")
	}
}

func TestJcodeInvalidThemeDoesNotPublishConfig(t *testing.T) {
	for _, input := range []string{
		"[display",
		"display = 'invalid'",
		"[display]\ntheme = 'dark'\ncolors = 'invalid'",
		"[display]\ntheme = 'light'\n[display.colors]\naccent = '#73d0ff'",
		"[display]\ntheme = 'dark'\n[display.colors]",
		"[display]\ntheme = 'dark'\n[display.colors]\naccent = '#123'",
		"[display]\ntheme = 'dark'\n[display.colors]\naccent = '#zzzzzz'",
	} {
		t.Run(input, func(t *testing.T) {
			home, root := jcodeFixture(t)
			data := filepath.Join(home, "data")
			t.Setenv("JCODE_INTEGRATIONS_DIR", data)
			put(t, filepath.Join(root, "config/jcode/theme.toml"), input)
			config := filepath.Join(home, ".jcode/config.toml")
			mcp := filepath.Join(home, ".jcode/mcp.json")
			put(t, config, "[display]\ntheme = 'light'\n")
			put(t, mcp, `{"servers":{"personal":{"command":"personal"}}}`)
			beforeConfig, beforeMCP := get(t, config), get(t, mcp)
			if err := jcodeApply(root); err == nil {
				t.Fatal("invalid theme must fail before publication")
			}
			if get(t, config) != beforeConfig || get(t, mcp) != beforeMCP {
				t.Fatal("invalid theme partially changed native settings")
			}
			if _, err := os.Stat(filepath.Join(data, "neo4j/private.env")); !os.IsNotExist(err) {
				t.Fatal("theme validation must happen before credential creation")
			}
		})
	}
}
