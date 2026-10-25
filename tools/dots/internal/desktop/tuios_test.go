package desktop

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopUsesTUIOSSocketOutsidePane(t *testing.T) {
	f := newFixture(t)
	// Short UNIX path, kept independent of the person's profile and sockets.
	dir, err := os.MkdirTemp("/tmp", "dots-mux-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "run/tuios/servers/dots/tuios.sock.herdr")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan error, 1)
	go func() {
		c, e := ln.Accept()
		if e != nil {
			done <- e
			return
		}
		defer c.Close()
		var request map[string]any
		if e = json.NewDecoder(c).Decode(&request); e != nil {
			done <- e
			return
		}
		done <- json.NewEncoder(c).Encode(map[string]any{"id": request["id"], "result": map[string]any{"snapshot": map[string]any{"version": "0.9.5+tuios", "workspaces": []map[string]any{{"workspace_id": "ws-check", "label": "tuios-check", "tab_count": 1, "pane_count": 1}}, "panes": []any{}}}})
	}()
	f.env = append(f.env, "DOTS_TUIOS_PROFILE="+dir)
	f.fake("herdr", allRule("must not use old Herdr", 1))
	f.fake("sesh", allRule("[]", 0))
	out, stderr, code := f.run("", "sessions", "list", "--json")
	if code != 0 {
		t.Fatalf("TUIOS default backend: %d %s %s", code, out, stderr)
	}
	var rows []session
	if json.Unmarshal([]byte(out), &rows) != nil || len(rows) != 1 || rows[0].Source != "tuios" || rows[0].Name != "tuios-check" {
		t.Fatalf("TUIOS inventory: %s", out)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestExplicitHerdrSocketWinsOverTUIOSContext(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", "/explicit/old-herdr.sock")
	t.Setenv("TUIOS_SOCKET", "/some/tuios.sock")
	if got := muxSocketPath(); got != "/explicit/old-herdr.sock" {
		t.Fatalf("explicit socket overwritten: %q", got)
	}
}

func TestRefsAcceptsTUIOSAndAlphanumericHerdrPaneIDs(t *testing.T) {
	for _, pane := range []string{"w8:p5J", "waeb6e46e3644:p4841bd637e8f"} {
		t.Run(pane, func(t *testing.T) {
			f := newFixture(t)
			dir, err := os.MkdirTemp("/tmp", "dots-refs-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			path := filepath.Join(dir, "tuios.sock.herdr")
			ln, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			go func() {
				for i := 0; i < 2; i++ {
					c, err := ln.Accept()
					if err != nil {
						return
					}
					var req map[string]any
					if json.NewDecoder(c).Decode(&req) != nil {
						c.Close()
						return
					}
					result := map[string]any{"pane": map[string]any{"cwd": f.root}}
					if req["method"] == "pane.read" {
						result = map[string]any{"read": map[string]any{"text": "https://example.com/tuios\n"}}
					}
					json.NewEncoder(c).Encode(map[string]any{"id": req["id"], "result": result})
					c.Close()
				}
			}()
			f.env = append(f.env, "TUIOS_SOCKET="+strings.TrimSuffix(path, ".herdr"))
			f.fake("herdr", allRule("must not call the retired backend", 1))
			out, stderr, code := f.run("", "refs", "--pane", pane, "--list")
			if code != 0 || out != "1. [url] https://example.com/tuios\n" {
				t.Fatalf("%d %q %q", code, out, stderr)
			}
		})
	}
}
