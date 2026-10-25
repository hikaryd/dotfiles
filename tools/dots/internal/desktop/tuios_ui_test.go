package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestTUIOSPreservesTerminalTransparencyAndSolidDock(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../.."))
	data, err := os.ReadFile(filepath.Join(root, "config/tuios/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Appearance struct {
			DockCompact    bool   `toml:"dock_compact"`
			Theme          string `toml:"theme"`
			Background     string `toml:"background"`
			PaneBackground string `toml:"pane_background"`
			DockBackground string `toml:"dock_background"`
			Sidebar        struct {
				Background string `toml:"background"`
			} `toml:"sidebar"`
		} `toml:"appearance"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Appearance.DockCompact {
		t.Error("TUIOS dock must not draw the unwanted horizontal separator")
	}
	if cfg.Appearance.Theme != "ayu-dots" {
		t.Fatalf("daily UI must keep Ayu, got %q", cfg.Appearance.Theme)
	}
	for name, value := range map[string]string{
		"canvas": cfg.Appearance.Background,
		"pane":   cfg.Appearance.PaneBackground,
	} {
		if value != "off" {
			t.Errorf("%s must leave default terminal cells unpainted so Rio/Ghostty transparency works: %q", name, value)
		}
	}
	for name, value := range map[string]string{
		"dock":    cfg.Appearance.DockBackground,
		"sidebar": cfg.Appearance.Sidebar.Background,
	} {
		if value != "theme" {
			t.Errorf("%s must keep legible Ayu chrome over the transparent terminal: %q", name, value)
		}
	}
}

func TestTUIOSPrefixKeepsTerminalInput(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../.."))
	data, err := os.ReadFile(filepath.Join(root, "config/tuios/config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Appearance struct {
			AutoEnterTerminalOnFocus string `toml:"auto_enter_terminal_on_focus"`
			DockModeSplitLetter      *bool  `toml:"dock_mode_split_letter"`
		} `toml:"appearance"`
		Startup struct {
			StartInTerminalMode bool `toml:"start_in_terminal_mode"`
		} `toml:"startup"`
		Keybindings struct {
			LeaderKey  string              `toml:"leader_key"`
			Repeat     []string            `toml:"repeat"`
			PrefixMode map[string][]string `toml:"prefix_mode"`
			Command    []struct {
				Key     string `toml:"key"`
				Type    string `toml:"type"`
				Command string `toml:"command"`
			} `toml:"command"`
		} `toml:"keybindings"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Keybindings.LeaderKey != "ctrl+a" {
		t.Error("the daily prefix must remain Ctrl+A")
	}
	if cfg.Keybindings.Repeat == nil || len(cfg.Keybindings.Repeat) != 0 {
		t.Error("prefix commands must be one-shot, only the explicit resize menu stays armed")
	}
	if !cfg.Startup.StartInTerminalMode {
		t.Error("an attached client must start ready to type into its pane")
	}
	if cfg.Appearance.AutoEnterTerminalOnFocus != "all" {
		t.Error("focusing a pane must return to terminal input, including Tab focus cycling")
	}
	if cfg.Appearance.DockModeSplitLetter == nil || *cfg.Appearance.DockModeSplitLetter {
		t.Error("the mode pill must not mislabel terminal input with the V/H split direction")
	}
	if keys, explicit := cfg.Keybindings.PrefixMode["prefix_exit_mode"]; !explicit || len(keys) != 0 {
		t.Error("the prefix must explicitly disable the inherited Esc-to-window-mode binding")
	}
	keys := cfg.Keybindings.PrefixMode["enter_terminal_mode"]
	if len(keys) != 1 || keys[0] != "esc" {
		t.Error("Ctrl+A then Esc must cancel to terminal input without forwarding Esc to the application")
	}
	if keys, explicit := cfg.Keybindings.PrefixMode["new_workspace"]; !explicit || len(keys) != 0 {
		t.Error("disable new_workspace: it switches to an empty workspace and drops terminal mode before creating its pane")
	}
	var newTerminalWorkspace bool
	for _, command := range cfg.Keybindings.Command {
		if command.Key != "prefix+c" {
			continue
		}
		newTerminalWorkspace = command.Type == "shell" &&
			strings.Contains(command.Command, `tmux new-window -t "$TUIOS_SESSION:"`) &&
			strings.Contains(command.Command, `-c "$TUIOS_ACTIVE_PANE_CWD"`)
	}
	if !newTerminalWorkspace {
		t.Error("Ctrl+A c must create the pane before switching workspaces, targeting the invoking session and preserving cwd")
	}
}
