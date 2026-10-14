package proxy

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiProxyRejectsRetiredInterfacesBeforeStartingAnything(t *testing.T) {
	for _, args := range [][]string{{"codex", "--print-env"}, {"pi", "--run-app"}, {"pi", "--install-codex-env"}, {"pi", "--unset-app-env"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("PATH", t.TempDir())
			var out, diagnostic bytes.Buffer
			if code := run("", args, strings.NewReader(""), &out, &diagnostic); code != 2 {
				t.Fatalf("retired interface accepted: %d %s", code, &diagnostic)
			}
			if _, err := os.Stat(filepath.Join(home, ".codex")); !os.IsNotExist(err) {
				t.Fatal("retired environment recreated")
			}
			if _, err := os.Stat(filepath.Join(home, ".config/xray-pi/xray.pid")); !os.IsNotExist(err) {
				t.Fatal("unexpected proxy process")
			}
		})
	}
}
