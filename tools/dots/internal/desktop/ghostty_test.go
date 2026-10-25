package desktop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Ghostty appends Nushell's --execute bootstrap to a command explicitly marked
// as Nushell. The outer command is a multiplexer, not Nu, so that bootstrap
// breaks the default GUI launch before any shell can start.
func TestGhosttyDoesNotInjectShellArgumentsIntoMultiplexer(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Ghostty GUI configuration is macOS-specific")
	}
	bin := "/Applications/Ghostty.app/Contents/MacOS/ghostty"
	if _, err := os.Stat(bin); err != nil {
		t.Skip("Ghostty is not installed")
	}
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../.."))
	configHome := t.TempDir()
	configDir := filepath.Join(configHome, "ghostty")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "config/ghostty/config"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config"), data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "+show-config")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+configHome)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Ghostty config load: %v: %s", err, out)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if key, value, ok := strings.Cut(line, " = "); ok {
			values[key] = value
		}
	}
	if !strings.Contains(values["command"], "/.local/bin/dots-terminal") {
		t.Fatalf("Ghostty did not load the managed multiplexer launcher: %q", values["command"])
	}
	if values["shell-integration"] != "none" {
		t.Fatalf("Ghostty would inject shell bootstrap arguments into TUIOS: shell-integration=%q", values["shell-integration"])
	}
}
