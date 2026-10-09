package desktop

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRefsExtraction(t *testing.T) {
	f := newFixture(t)
	file := filepath.Join(f.root, "source name.go")
	f.write(file, "fixture", 0600)
	text := "\x1b]0;https://hidden.example\a\x1b[31mhttps://ok.example/a).\x1b[0m https://ok.example/a\nhttps://user:pass@bad.example https://bad.example/%0a\nrequest-id: abc-123 trace_id='abc-123'\n12345678-1234-abcd-1234-123456789abc\nFile \"source name.go\", line 12\n"
	items, e := extractRefs(text, f.root)
	if e != nil {
		t.Fatal(e)
	}
	want := []reference{{Kind: "url", Value: "https://ok.example/a"}, {Kind: "id", Value: "abc-123"}, {Kind: "id", Value: "12345678-1234-abcd-1234-123456789abc"}, {Kind: "file", Value: file + ":12", Path: file, Line: 12}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("got %+v want %+v", items, want)
	}
	f.write(filepath.Join(f.root, "simple.go"), "x", 0600)
	items, e = extractRefs("simple.go:7:9 simple.go:7\n", f.root)
	if e != nil || len(items) != 1 || items[0].Line != 7 {
		t.Fatalf("file refs %+v %v", items, e)
	}
}
func TestRefsSanitization(t *testing.T) {
	for _, seq := range []string{"\x1b]evil", "\x1bPevil", "\x1b_evil", "\u009devil", "\u0090evil\u009c"} {
		if s := sanitize("safe " + seq); s != "safe " {
			t.Fatalf("unsafe terminal sequence %q", s)
		}
	}
	for _, s := range []string{"https://x/%20", "https://x/%1b", "https://x/%E2%80%AE", "https://user@x/", "file:///x", "https://x\\y", "https:///path"} {
		if safeURL(s) {
			t.Errorf("unsafe URL accepted %q", s)
		}
	}
	if !safeURL("https://example.com/a?q=hello%2Fworld") {
		t.Fatal("safe URL rejected")
	}
}
func TestRefsCLIStdinAndCapture(t *testing.T) {
	f := newFixture(t)
	out, err, code := f.run("https://example.com\nrequest-id: abc", "refs", "--stdin", "--json")
	var refs []reference
	if code != 0 || json.Unmarshal([]byte(out), &refs) != nil || len(refs) != 2 {
		t.Fatalf("%d %q %q", code, out, err)
	}
	f.fake("tmux", fakeRule{Args: []string{"display-message", "-p", "-t", "%9", "#{pane_current_path}"}, Output: f.root + "\n"}, fakeRule{Args: []string{"capture-pane", "-p", "-J", "-S", "-25", "-t", "%9"}, Output: "https://example.com\n"})
	out, err, code = f.run("", "refs", "--pane", "%9", "--lines", "25", "--list")
	if code != 0 || out != "1. [url] https://example.com\n" {
		t.Fatalf("%d %q %q", code, out, err)
	}
	_, _, code = f.run("", "refs", "--pane", "%9;bad", "--list")
	if code != 1 {
		t.Fatal("invalid pane accepted")
	}
}
func TestRefsPickerDisablesFZFHooks(t *testing.T) {
	f := newFixture(t)
	f.fake("fzf", allRule("not a row", 0))
	f.env = append(f.env, "FZF_DEFAULT_OPTS=--preview=unsafe", "FZF_DEFAULT_OPTS_FILE=/fixture/unsafe")
	_, _, code := f.run("https://example.com", "refs", "--stdin")
	if code != 0 {
		t.Fatal(code)
	}
	calls := f.calls()
	if len(calls) != 1 || calls[0].Env["FZF_DEFAULT_OPTS"] != "" || calls[0].Env["FZF_DEFAULT_OPTS_FILE"] != "/dev/null" {
		t.Fatalf("picker env %+v", calls)
	}
}
func TestRefsSizeLimit(t *testing.T) {
	f := newFixture(t)
	out, _, code := f.run(strings.Repeat("x", maxBytes+1), "refs", "--stdin", "--json")
	if code != 1 || out != "" {
		t.Fatal("oversized input accepted")
	}
}
