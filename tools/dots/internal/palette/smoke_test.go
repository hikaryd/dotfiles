package palette

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Runs every generator against a copy of the real configs, so a config edit
// that breaks a generator fails here instead of on the next theme switch.
func TestGeneratorsRunOnRepositoryConfigs(t *testing.T) {
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"dark", "mirage"} {
		root := t.TempDir()
		for _, file := range []string{"palette/ayu.toml", "config/ghostty/config", "config/starship/starship.toml", "config/tmux/tmux.conf", "config/lazygit/config.yml"} {
			data, err := os.ReadFile(filepath.Join(repo, file))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, file)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		// Generators write these without creating the directory.
		for _, dir := range []string{"config/nushell", "config/yazi"} {
			if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		var stderr bytes.Buffer
		if code := run(root, []string{"--variant", variant}, &bytes.Buffer{}, &stderr); code != 0 {
			t.Fatalf("%s: exit %d: %s", variant, code, stderr.String())
		}
	}
}
