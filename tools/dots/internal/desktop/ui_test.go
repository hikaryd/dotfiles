package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUIHelpAndPreview(t *testing.T) {
	for _, env := range [][]string{{"TERM=xterm-256color", "FORCE_COLOR=1"}, {"TERM=xterm-256color", "NO_COLOR="}, {"TERM=dumb"}} {
		f := newFixture(t)
		f.env = append(f.env, env...)
		// Neither help nor preview needs to read even an unusable profile.
		if err := os.MkdirAll(filepath.Join(f.home, ".config/dots/style"), 0700); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"--help"}, {"menu", "--help"}, {"menu", "--preview"}, {"menu", "--plain", "--preview"}} {
			out, err, code := f.run("", args...)
			if code != 0 || err != "" || !strings.HasPrefix(out, "dots · ") || strings.Contains(out, "\x1b") || strings.Contains(out, f.base) || strings.Contains(out, filepath.Base(f.root)) {
				t.Fatalf("%v: %d %q %q", args, code, out, err)
			}
			for _, hint := range []string{"↑/↓", "Enter", "Esc"} {
				if !strings.Contains(out, hint) {
					t.Fatalf("%v missing %s", args, hint)
				}
			}
			if args[0] == "--help" || has("--preview", args...) {
				for _, group := range []string{"Работа", "Инструменты", "Проверки", "Настройка"} {
					if !strings.Contains(out, group) {
						t.Fatalf("%v missing group %s", args, group)
					}
				}
			}
			if has("--preview", args...) {
				for _, choice := range menuChoices() {
					if !strings.Contains(out, choice[0]+" · "+choice[1]) {
						t.Fatalf("preview missing actual choice %v", choice)
					}
				}
			}
		}
		if len(f.calls()) != 0 {
			t.Fatalf("static UI executed commands: %+v", f.calls())
		}
	}
}

func TestUIMenuInvalidArguments(t *testing.T) {
	f := newFixture(t)
	for _, args := range [][]string{{"--plain"}, {"--preview", "extra"}, {"--preview=true"}, {"--json"}} {
		out, _, code := f.run("", append([]string{"menu"}, args...)...)
		if code != 2 || out != "" {
			t.Fatalf("invalid menu args %v: %d %q", args, code, out)
		}
	}
	if len(f.calls()) != 0 {
		t.Fatal("invalid arguments opened picker")
	}
}

func TestUIMachineOutputExact(t *testing.T) {
	f := newFixture(t)
	f.env = append(f.env, "TERM=xterm-256color", "CLICOLOR_FORCE=1")
	f.registry()
	f.fake("tmux", fakeRule{Args: []string{"list-panes"}, Prefix: true, Output: ""})
	for _, tc := range []struct {
		args        []string
		input, want string
	}{
		{[]string{"projects", "list", "--json"}, "", "[]\n"},
		{[]string{"refs", "--stdin", "--json"}, "https://example.com", "[{\"kind\":\"url\",\"value\":\"https://example.com\"}]\n"},
		{[]string{"clip", "compact", "--stdin", "--stdout"}, "{\"x\": 1}", "{\"x\":1}\n"},
	} {
		out, err, code := f.run(tc.input, tc.args...)
		if code != 0 || err != "" || out != tc.want {
			t.Fatalf("%v: %d %q %q; want %q", tc.args, code, out, err, tc.want)
		}
	}
	if calls := f.calls(); len(calls) != 1 || calls[0].Name != "tmux" || calls[0].Args[0] != "list-panes" {
		t.Fatalf("machine mode executed extra commands: %+v", calls)
	}
}
