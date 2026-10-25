package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"dots/native/internal/mux"
)

var styleModes = []string{"glass", "focus", "presentation"}

func loadStyle() (string, error) {
	b, e := os.ReadFile(filepath.Join(configHome(), "dots/style"))
	if e != nil && !os.IsNotExist(e) {
		return "", errors.New("style file unavailable")
	}
	s := strings.TrimSpace(string(b))
	if !has(s, styleModes...) {
		return "glass", nil
	}
	return s, nil
}

// Notification hooks deliberately tolerate unavailable optional style state.
func readStyle() string {
	s, err := loadStyle()
	if err != nil {
		return "glass"
	}
	return s
}

var presentation = [][2]string{{"status-left", "#[fg=#bfbdb6] PRESENTATION "}, {"window-status-format", " #I terminal "}, {"window-status-current-format", "#[fg=#73d0ff,bold] #I terminal "}}

const snapshotOption = "@dots-style-snapshot"

func tmuxStyle(mode string) error {
	r, e := captureCommand(4*time.Second, maxBytes, "", nil, mux.Argv("list-sessions")...)
	if e != nil {
		if !available(mux.Argv()[0]) {
			return nil
		}
		return e
	}
	if r.code != 0 {
		return nil
	}
	show := func(key string) (string, error) {
		r, e := captureCommand(4*time.Second, maxBytes, "", nil, mux.Argv("show-options", "-gqv", key)...)
		if e != nil || r.code != 0 {
			return "", errors.New("tmux show-options failed")
		}
		return strings.TrimRight(r.out, "\n"), nil
	}
	set := func(k, v string) error {
		r, e := captureCommand(4*time.Second, maxBytes, "", nil, mux.Argv("set-option", "-g", k, v)...)
		if e != nil || r.code != 0 {
			return errors.New("tmux set-option failed")
		}
		return nil
	}
	raw, e := show(snapshotOption)
	if e != nil {
		return e
	}
	saved := map[string]string{}
	if raw != "" {
		if json.Unmarshal([]byte(raw), &saved) != nil || saved == nil {
			return errors.New("invalid tmux style snapshot")
		}
	}
	if mode == "presentation" {
		if len(saved) == 0 {
			for _, p := range presentation {
				v, e := show(p[0])
				if e != nil {
					return e
				}
				saved[p[0]] = v
			}
			b, e := jsonBytes(saved, false)
			if e != nil {
				return e
			}
			if e = set(snapshotOption, string(b)); e != nil {
				return e
			}
		}
		for _, p := range presentation {
			if e = set(p[0], p[1]); e != nil {
				return e
			}
		}
	} else if len(saved) > 0 {
		for _, p := range presentation {
			if previous, ok := saved[p[0]]; ok {
				current, e := show(p[0])
				if e != nil {
					return e
				}
				if current == p[1] {
					if e = set(p[0], previous); e != nil {
						return e
					}
				}
			}
		}
		r, e := captureCommand(4*time.Second, maxBytes, "", nil, mux.Argv("set-option", "-gu", snapshotOption)...)
		if e != nil || r.code != 0 {
			return errors.New("tmux snapshot reset failed")
		}
	}
	return nil
}
func tmuxStyleRun(args []string) int {
	if len(args) != 1 {
		return usageError("tmux-style")
	}
	mode := args[0]
	if mode == "--current" {
		b, e := os.ReadFile(filepath.Join(configHome(), "dots/style"))
		if os.IsNotExist(e) {
			mode = "glass"
		} else if e != nil {
			return fail("tmux style", errors.New("style unreadable"))
		} else {
			mode = strings.TrimSpace(string(b))
		}
	}
	if !has(mode, styleModes...) {
		return usageError("tmux-style")
	}
	if e := tmuxStyle(mode); e != nil {
		return fail("tmux style", e)
	}
	return 0
}
func ghosttyReload() int {
	if runtime.GOOS != "darwin" {
		return 0
	}
	r, e := captureCommand(5*time.Second, 4096, "", nil, "pgrep", "-x", "ghostty")
	if e != nil {
		return fail("Ghostty", e)
	}
	if r.code != 0 {
		return 0
	}
	script := `tell application "Ghostty"
  if (count of windows) is 0 then return false
  set targetTerminal to focused terminal of selected tab of front window
  if not (perform action "reload_config" on targetTerminal) then return false
  repeat with w in windows
    repeat with t in terminals of w
      perform action "reset_font_size" on t
    end repeat
  end repeat
  return true
end tell`
	r, e = captureCommand(5*time.Second, 4096, "", nil, "osascript", "-e", script)
	if e == nil && r.code == 0 && strings.TrimSpace(r.out) == "true" {
		fmt.Println("Ghostty: configuration reloaded, font size reset to profile default.")
		return 0
	}
	fmt.Fprintln(os.Stderr, "Ghostty: profile saved; live reload unavailable (Cmd+Shift+, then Cmd+0).")
	return 1
}
func styleRun(args []string) int {
	mode := "status"
	if len(args) == 1 {
		mode = args[0]
	} else if len(args) > 1 {
		return usageError("style")
	}
	if mode == "status" {
		style, err := loadStyle()
		if err != nil {
			return fail("dots style", err)
		}
		fmt.Println(style)
		return 0
	}
	if !has(mode, styleModes...) {
		return usageError("style")
	}
	override := "# Managed by dots style; glass inherits the base configuration.\n"
	if mode != "glass" {
		override += "background-opacity = 1\nbackground-blur = 0\ncustom-shader =\ncustom-shader-animation = false\n"
	}
	if mode == "presentation" {
		override += "font-size = 20\ntitle = Presentation\nforeground = f5f5f5\nselection-background = 405366\nselection-foreground = ffffff\n"
	}
	config := filepath.Join(configHome(), "dots")
	if e := atomicWrite(filepath.Join(config, "ghostty-style.conf"), []byte(override)); e != nil {
		return fail("dots style", errors.New("cannot save style"))
	}
	if e := atomicWrite(filepath.Join(config, "style"), []byte(mode+"\n")); e != nil {
		return fail("dots style", errors.New("cannot save style"))
	}
	code := 0
	if e := tmuxStyle(mode); e != nil {
		code = fail("tmux style", e)
	}
	reload := ghosttyReload()
	if code == 0 {
		code = reload
	}
	fmt.Printf("%s: saved.\n", mode)
	fmt.Println("Neovim: applies on FocusGained. Presentation does not hide buffer or scrollback contents.")
	return code
}
func herdrStatus() int {
	style, err := loadStyle()
	if err != nil {
		return 1
	}
	if style == "presentation" {
		fmt.Println("PRESENTATION")
		return 0
	}
	id := os.Getenv("HERDR_ACTIVE_WORKSPACE_ID")
	if id == "" {
		return 0
	}
	r, e := herdrCall("workspace.get", map[string]any{"workspace_id": id}, "workspace", "get", id)
	if e != nil {
		return 1
	}
	label := str(object(r["workspace"])["label"])
	label = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, label)
	label = truncate(label, 22)
	if label != "" {
		fmt.Println(" " + label)
	}
	return 0
}
