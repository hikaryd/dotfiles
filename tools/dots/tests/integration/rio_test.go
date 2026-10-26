package integration_test

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
func TestLegacyRioShellStartsOutsideInheritedMuxSession(t *testing.T) {
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
	markers := []string{"HERDR_ENV", "HERDR_BIN_PATH", "HERDR_SOCKET_PATH", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID", "HERDR_ACTIVE_PANE_ID", "TMUX"}
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
			adapter, err := os.ReadFile(filepath.Join(root, "scripts/dots-terminal"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bin, "dots-terminal"), adapter, 0755); err != nil {
				t.Fatal(err)
			}
			rmuxConfig := filepath.Join(home, ".config/rmux")
			if err := os.MkdirAll(rmuxConfig, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(rmuxConfig, "rmux.conf"), nil, 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bin, "rmux"), []byte("#!/bin/sh\n[ \"${1:-}\" = has-session ] && exit 1\nprintf 'DOTS_MUX=rmux\\n'\nexec /usr/bin/env\n"), 0755); err != nil {
				t.Fatal(err)
			}
			// The full race suite starts many instrumented subprocesses together.
			// Keep a bounded deadline without treating host scheduling as a hang.
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
			if env["DOTS_MUX"] != "rmux" {
				t.Fatal("Legacy Rio config must use the current rmux-only launcher")
			}
		})
	}
}

func TestRioUsesCalibratedBlurWithoutGlassRim(t *testing.T) {
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
			Blur         any     `toml:"blur"`
		} `toml:"window"`
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	// Restore the pre-experiment two-layer blur calibration. Native clear glass
	// draws a bright rim over wallpaper even when the mux paints a dark canvas.
	if config.Window.Blur != true || config.Window.Opacity != 0.75 || config.Window.OpacityCells {
		t.Fatalf("Rio must keep ordinary calibrated blur, no glass material, and solid UI fills: %+v", config.Window)
	}
	// Powerline rounded caps are opaque foreground glyphs. Their adjacent
	// background fills must also stay opaque, while the Herdr base canvas
	// must use the terminal default to keep the wallpaper visible.
	data, err = os.ReadFile(filepath.Join(root, "config/herdr/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var herdr struct {
		Theme struct {
			Custom struct {
				PanelBg string `toml:"panel_bg"`
			} `toml:"custom"`
		} `toml:"theme"`
	}
	if err := toml.Unmarshal(data, &herdr); err != nil {
		t.Fatal(err)
	}
	if herdr.Theme.Custom.PanelBg != "reset" {
		t.Fatalf("Herdr must use the translucent terminal background: panel_bg=%q", herdr.Theme.Custom.PanelBg)
	}
}

func TestRioCursorTrailIsVisibleAndShort(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../.."))
	data, err := os.ReadFile(filepath.Join(root, "config/rio/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Effects struct {
			Trail   bool    `toml:"trail-cursor"`
			Color   string  `toml:"trail-cursor-color"`
			Opacity float64 `toml:"trail-cursor-opacity"`
			Decay   [2]int  `toml:"trail-cursor-decay"`
		} `toml:"effects"`
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	effects := config.Effects
	if !effects.Trail || effects.Color != "#73d0ff" || effects.Opacity <= 0 || effects.Opacity > 0.4 || effects.Decay[0] <= 0 || effects.Decay[0] > effects.Decay[1] || effects.Decay[1] > 140 {
		t.Fatalf("Rio needs a visible short trail, not a long full-screen animation: %+v", effects)
	}
}

func TestRioRendererAvoidsContinuousAndOccludedRedraw(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../.."))
	data, err := os.ReadFile(filepath.Join(root, "config/rio/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Renderer struct {
			Backend                string `toml:"backend"`
			Strategy               string `toml:"strategy"`
			DisableOccludedRender  bool   `toml:"disable-occluded-render"`
			DisableUnfocusedRender bool   `toml:"disable-unfocused-render"`
		} `toml:"renderer"`
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config.Renderer.Backend != "metal" || config.Renderer.Strategy != "events" || !config.Renderer.DisableOccludedRender || config.Renderer.DisableUnfocusedRender {
		t.Fatalf("Rio must render on demand, skip covered windows and keep the visible second monitor updating: %+v", config.Renderer)
	}
}

func TestRioDoesNotReserveHiddenGPUTabStrip(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../.."))
	data, err := os.ReadFile(filepath.Join(root, "config/rio/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Navigation struct {
			Mode string `toml:"mode"`
		} `toml:"navigation"`
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	// On macOS Tab reserves 38pt even when hidden, Plain reserves 26pt.
	// NativeTab with no unified titlebar uses only the configured margin.
	if config.Navigation.Mode != "nativetab" {
		t.Fatalf("Rio reserves extra top chrome: navigation mode %q", config.Navigation.Mode)
	}
}
