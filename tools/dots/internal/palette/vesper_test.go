package palette

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestRepositoryVesperTerminals(t *testing.T) {
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	root := fixture(t)
	mustWrite(t, filepath.Join(root, "palette/ayu.toml"), mustRead(t, filepath.Join(repo, "palette/ayu.toml")))
	var document struct {
		Dark palette `toml:"dark"`
	}
	if err := toml.Unmarshal(mustRead(t, filepath.Join(root, "palette/ayu.toml")), &document); err != nil {
		t.Fatal(err)
	}
	p := document.Dark
	if p["editor"]["bg"] != "#101010" || p["syntax"]["string"] != "#99ffe4" || p["common"]["accent"] != "#ffc799" {
		t.Fatal("repository palette is not Vesper")
	}
	var out, stderr bytes.Buffer
	if code := run(root, nil, &out, &stderr); code != 0 {
		t.Fatalf("generation: %s", &stderr)
	}
	before := snapshot(t, root)
	ghost := before["config/ghostty/config"]
	rex := before["config/rex/ayu-dark"]
	var rio struct {
		Colors map[string]string `toml:"colors"`
	}
	if err := toml.Unmarshal([]byte(before["config/rio/themes/ayu.toml"]), &rio); err != nil {
		t.Fatal(err)
	}
	keys := []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white", "bright_black", "bright_red", "bright_green", "bright_yellow", "bright_blue", "bright_magenta", "bright_cyan", "bright_white"}
	for i, key := range keys {
		color := p["terminal"][key]
		line := fmt.Sprintf("palette = %d=%s\n", i, color)
		if !strings.Contains(ghost, line) || !strings.Contains(rex, line) {
			t.Fatalf("terminal %d differs from source", i)
		}
		rioKey := strings.Replace(key, "bright_", "light-", 1)
		if rio.Colors[rioKey] != color {
			t.Fatalf("Rio %s = %q, want %q", rioKey, rio.Colors[rioKey], color)
		}
	}
	if strings.Contains(ghost, "theme = Ayu") || !strings.Contains(ghost, "background = 101010\n") || !strings.Contains(rex, "foreground = ffffff\n") {
		t.Fatal("stale terminal theme")
	}
	out.Reset()
	stderr.Reset()
	if code := run(root, nil, &out, &stderr); code != 0 {
		t.Fatalf("repeat: %s", &stderr)
	}
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("generation is not idempotent")
	}
	if _, err := os.Stat(filepath.Join(repo, "config/rex/ayu-dark")); err != nil {
		t.Fatal(err)
	}
}
