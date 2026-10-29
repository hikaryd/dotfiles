package rmuxstate

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIHelpAndDisabledStatusDoNotCreateState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	var out, diagnostic bytes.Buffer
	for _, args := range [][]string{{"--help"}, {"status"}, {"bogus"}} {
		code := Run(args, &out, &diagnostic)
		if args[0] == "bogus" && code != 2 {
			t.Fatal(code)
		}
		if args[0] != "bogus" && code != 0 {
			t.Fatal(code, diagnostic.String())
		}
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatal("status created state")
	}
}
func TestInstallPrivateEscapedLaunchAgentsAndEnableLast(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home & <private>")
	os.Mkdir(home, 0700)
	helper := filepath.Join(home, "dots tools")
	bin := filepath.Join(home, "rmux")
	config := filepath.Join(home, "rmux.conf")
	for _, p := range []string{helper, bin} {
		os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0700)
	}
	os.WriteFile(config, nil, 0600)
	s, e := Open(filepath.Join(home, "state"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	r := Runtime{Home: home, Bin: bin, Config: config, Socket: filepath.Join(home, "socket"), Store: s}
	if e = r.Install(helper); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"restore", "save"} {
		p := filepath.Join(home, "Library", "LaunchAgents", "local.dots.rmux-state-"+kind+".plist")
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		d := xml.NewDecoder(bytes.NewReader(b))
		for {
			_, e = d.Token()
			if e != nil {
				break
			}
		}
		if e.Error() != "EOF" {
			t.Fatal(e)
		}
		text := string(b)
		if !strings.Contains(text, "&amp;") || !strings.Contains(text, "&lt;") || strings.Contains(text, "KeepAlive") {
			t.Fatal(text)
		}
		if kind == "save" && !strings.Contains(text, "<integer>60</integer>") {
			t.Fatal(text)
		}
		if kind == "restore" && !strings.Contains(text, "<key>RunAtLoad</key><true/>") {
			t.Fatal(text)
		}
		i, _ := os.Stat(p)
		if i.Mode().Perm() != 0600 {
			t.Fatal(i.Mode())
		}
	}
	enabled, e := s.read("enabled")
	if e != nil || string(enabled) != "v1\n"+helper+"\n" {
		t.Fatalf("marker %q %v", enabled, e)
	}
	os.Remove(filepath.Join(s.Dir, "enabled"))
	if e = r.Install("/missing/helper"); e == nil {
		t.Fatal("invalid helper enabled install")
	}
	if _, e = s.read("enabled"); e == nil {
		t.Fatal("enabled failed install")
	}
}
