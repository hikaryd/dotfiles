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

	"github.com/pelletier/go-toml/v2"
)

// Exercise the actual Rio shell command, not a copy of its environment filter.
// GUI apps opened from a Herdr pane must not masquerade as another Herdr pane.
func TestRioShellStartsOutsideInheritedHerdrSession(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../.."))
	data, err := os.ReadFile(filepath.Join(root, "config/rio/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Shell struct {
			Program string   `toml:"program"`
			Args    []string `toml:"args"`
		} `toml:"shell"`
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	markers := []string{"HERDR_ENV", "HERDR_BIN_PATH", "HERDR_SOCKET_PATH", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID"}
	for _, inherited := range []bool{false, true} {
		name := "clean"
		if inherited {
			name = "opened-from-herdr"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			bin := filepath.Join(home, ".local/bin")
			if err := os.MkdirAll(bin, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bin, "dots-nu"), []byte("#!/bin/sh\nexec /usr/bin/env\n"), 0755); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, config.Shell.Program, config.Shell.Args...)
			cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin", "TERM=rio", "KEEP_ME=unchanged"}
			if inherited {
				for _, marker := range markers {
					cmd.Env = append(cmd.Env, marker+"=inherited")
				}
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Rio shell launch: %v: %s", err, out)
			}
			env := map[string]string{}
			for _, line := range strings.Split(string(out), "\n") {
				if key, value, ok := strings.Cut(line, "="); ok {
					env[key] = value
				}
			}
			for _, marker := range markers {
				if _, exists := env[marker]; exists {
					t.Errorf("outer Rio shell inherited %s", marker)
				}
			}
			if env["KEEP_ME"] != "unchanged" || env["TERM"] != "rio" || env["HOME"] != home {
				t.Fatal("Rio shell discarded unrelated environment")
			}
		})
	}
}

func TestRioAppliesOpacityToTUIBackgrounds(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../.."))
	data, err := os.ReadFile(filepath.Join(root, "config/rio/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Window struct {
			Opacity      float64 `toml:"opacity"`
			OpacityCells bool    `toml:"opacity-cells"`
		} `toml:"window"`
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config.Window.Opacity != 0.94 || !config.Window.OpacityCells {
		t.Fatalf("TUI backgrounds must use the Ghostty opacity: got %+v", config.Window)
	}
}
