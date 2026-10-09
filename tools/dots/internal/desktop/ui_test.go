package desktop

import (
	"os"
	"path/filepath"
	"reflect"
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
	f.fake("gum", allRule("", 0))
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

func menuLabel(key string) string {
	for _, c := range menuChoices() {
		if c[0] == key {
			return c[0] + " · " + c[1]
		}
	}
	return ""
}

func TestUIMenuWhitelistBothSelectors(t *testing.T) {
	for _, picker := range []string{"gum", "fzf"} {
		for _, tc := range []struct {
			output string
			code   int
		}{
			{"", 0}, {"", 130}, {menuLabel("git") + "\n", 130},
			{"git\n", 0}, {"$(touch unsafe)\n", 0},
			{menuLabel("git") + "; touch unsafe\n", 0},
			{menuLabel("git") + "\n" + menuLabel("ports") + "\n", 0},
		} {
			f := newFixture(t)
			f.env = append(f.env, "FZF_DEFAULT_OPTS=--bind=start:execute(unsafe)", "FZF_DEFAULT_OPTS_FILE=/unsafe")
			f.fake(picker, allRule(tc.output, tc.code))
			out, err, code := f.run("", "menu")
			calls := f.calls()
			if code != 0 || out != "" || err != "" || len(calls) != 1 || calls[0].Name != picker {
				t.Fatalf("%s %q: %d %q %q %+v", picker, tc.output, code, out, err, calls)
			}
			if calls[0].Env["FZF_DEFAULT_OPTS"] != "" || calls[0].Env["FZF_DEFAULT_OPTS_FILE"] != "/dev/null" {
				t.Fatal("menu inherited executable fzf hooks")
			}
		}
	}
}

func TestUIMenuKnownDispatchAndNestedCancel(t *testing.T) {
	for _, picker := range []string{"gum", "fzf"} {
		for _, action := range []string{"git", "ports", "help", "style", "config", "kube"} {
			f := newFixture(t)
			f.fake(picker, fakeRule{Args: []string{}, Prefix: true, Outputs: []string{menuLabel(action) + "\n", "injected\n"}})
			f.fake("lazygit", allRule("git fixture\n", 0))
			f.fake("lsof", allRule("ports fixture\n", 0))
			out, err, code := f.run("", "menu")
			if code != 0 || err != "" {
				t.Fatalf("%s %s: %d %q", picker, action, code, err)
			}
			calls := f.calls()
			switch action {
			case "git":
				if len(calls) != 2 || calls[1].Name != "lazygit" || len(calls[1].Args) != 0 || out != "git fixture\n" {
					t.Fatalf("git dispatch: %+v %q", calls, out)
				}
			case "ports":
				if len(calls) != 2 || calls[1].Name != "lsof" || !reflect.DeepEqual(calls[1].Args, []string{"-nP", "-iTCP", "-sTCP:LISTEN"}) || out != "ports fixture\n" {
					t.Fatalf("ports dispatch: %+v %q", calls, out)
				}
			case "help":
				if len(calls) != 1 || !strings.Contains(out, "Примеры") {
					t.Fatalf("help dispatch: %+v %q", calls, out)
				}
			default:
				if len(calls) != 2 || calls[1].Name != picker || out != "" {
					t.Fatalf("nested cancel: %+v %q", calls, out)
				}
			}
		}
	}
}

func TestUIMenuContextPrivacy(t *testing.T) {
	for _, profile := range []string{"glass", "presentation", "unreadable"} {
		f := newFixture(t)
		p := filepath.Join(f.home, ".config/dots/style")
		if profile == "unreadable" {
			if err := os.MkdirAll(p, 0700); err != nil {
				t.Fatal(err)
			}
		} else {
			f.write(p, profile, 0600)
		}
		f.fake("gum", allRule("", 130))
		_, err, code := f.run("", "menu")
		if code != 0 || err != "" || len(f.calls()) != 1 {
			t.Fatalf("%d %q %+v", code, err, f.calls())
		}
		args := strings.Join(f.calls()[0].Args, "\n")
		if strings.Contains(args, f.base) || strings.Contains(args, filepath.Base(f.root)) != (profile == "glass") {
			t.Fatalf("context privacy %s: %q", profile, args)
		}
	}
}

func TestUISelectorStyleAndData(t *testing.T) {
	gum := []string{"filter", "--no-sort", "--limit", "1", "--placeholder", "Fixture"}
	fzf := []string{"--no-multi", "--prompt", "Fixture > "}
	g, f := selectorArgs(gum, fzf, true)
	for _, pair := range [][2]string{{"--indicator.foreground", ayuAccent}, {"--text.foreground", ayuText}, {"--match.foreground", ayuSuccess}, {"--placeholder.foreground", ayuMuted}, {"--prompt.foreground", ayuAccent}, {"--limit", "1"}, {"--padding", "0 1"}} {
		if !strings.Contains(strings.Join(g, "\n"), pair[0]+"\n"+pair[1]) {
			t.Fatalf("missing gum style/limit %v: %v", pair, g)
		}
	}
	for _, color := range []string{ayuAccent, ayuText, ayuMuted, ayuBorder, ayuSuccess, ayuWarning} {
		if !strings.Contains(strings.Join(f, "\n"), color) {
			t.Fatalf("missing fzf color %s", color)
		}
	}
	if !reflect.DeepEqual(gum, []string{"filter", "--no-sort", "--limit", "1", "--placeholder", "Fixture"}) || len(fzf) != 3 {
		t.Fatal("mutated caller flags")
	}
	if got := ayu("dots", ayuAccent, true); got != "\x1b[38;2;115;208;255mdots\x1b[0m" {
		t.Fatalf("Ayu accent: %q", got)
	}
	for _, picker := range []string{"gum", "fzf"} {
		fixture := newFixture(t)
		fixture.env = append(fixture.env, "NO_COLOR=1", "TERM=xterm-256color")
		fixture.fake(picker, allRule("injected", 0))
		out, err, code := fixture.run("https://example.com", "refs", "--stdin")
		if code != 0 || out != "" || err != "" || len(fixture.calls()) != 1 {
			t.Fatalf("refs %d %q %q", code, out, err)
		}
		call := fixture.calls()[0]
		if call.Input != "1. [url] https://example.com\n" || !has("--header", call.Args...) || strings.Contains(strings.Join(call.Args, " "), "#") {
			t.Fatalf("changed data or colored NO_COLOR selector: %+v", call)
		}
		if picker == "gum" && call.Args[0] != "filter" {
			t.Fatal("refs is not search-first")
		}
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
