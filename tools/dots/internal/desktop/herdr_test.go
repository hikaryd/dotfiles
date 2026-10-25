package desktop

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestHerdrSocketProtocolAndAlphanumericRefs(t *testing.T) {
	f := newFixture(t)
	dir, e := os.MkdirTemp("/tmp", "desktop-socket-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "s")
	ln, e := net.Listen("unix", path)
	if e != nil {
		if errors.Is(e, syscall.EPERM) || errors.Is(e, syscall.EACCES) {
			t.Skipf("local fixture socket blocked by sandbox: %v", e)
		}
		t.Fatalf("local fixture socket unavailable in sandbox: %v", e)
	}
	defer ln.Close()
	requests := make(chan map[string]any, 2)
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 2; i++ {
			c, e := ln.Accept()
			if e != nil {
				done <- e
				return
			}
			c.SetDeadline(time.Now().Add(3 * time.Second))
			var req map[string]any
			e = json.NewDecoder(bufio.NewReader(c)).Decode(&req)
			if e != nil {
				c.Close()
				done <- e
				return
			}
			requests <- req
			result := map[string]any{}
			if i == 0 {
				result["pane"] = map[string]any{"cwd": f.root}
			} else {
				result["read"] = map[string]any{"text": "https://fixture.example\n"}
			}
			e = json.NewEncoder(c).Encode(map[string]any{"id": req["id"], "result": result})
			c.Close()
			if e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
	f.env = append(f.env, "HERDR_SOCKET_PATH="+path)
	out, err, code := f.run("", "refs", "--pane", "w8:p5J", "--lines", "17", "--list")
	if code != 0 || out != "1. [url] https://fixture.example\n" {
		t.Fatalf("socket refs %d %s %s", code, out, err)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	first, second := <-requests, <-requests
	if first["method"] != "pane.get" || second["method"] != "pane.read" {
		t.Fatalf("methods %+v %+v", first, second)
	}
	want := map[string]any{"pane_id": "w8:p5J", "source": "recent_unwrapped", "lines": float64(17), "strip_ansi": true, "format": "text"}
	if !reflect.DeepEqual(second["params"], want) {
		t.Fatalf("params %+v", second)
	}
	if len(f.calls()) != 0 {
		t.Fatal("socket path spawned herdr")
	}
}
func TestHerdrErrorDoesNotDiscloseResponse(t *testing.T) {
	f := newFixture(t)
	f.fake("herdr", allRule(`{"error":{"message":"fixture-secret"}}`, 0))
	f.env = append(f.env, "HERDR_ENV=1")
	out, err, code := f.run("", "projects", "list", "--json")
	if code != 1 || out != "" || err == "" {
		t.Fatalf("%d %q %q", code, out, err)
	}
	if containsSecret(err) {
		t.Fatal("response leaked")
	}
}
func containsSecret(s string) bool {
	return strings.Contains(s, "fixture-secret")
}
