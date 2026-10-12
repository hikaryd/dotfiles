package proxy

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dots/native/internal/testpty"
)

func proxyTerminal(t *testing.T, home string, args ...string) *testpty.CLI {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = []string{"HOME=" + home, "PATH=" + t.TempDir(), "DOTS_PROXY_CLI=1", "NO_COLOR=1", "TERM=xterm-256color", "GORACE=atexit_sleep_ms=0"}
	return testpty.Start(t, cmd)
}
func TestNativeProxySecretConfiguration(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		home := t.TempDir()
		p := proxyTerminal(t, home, "codex", "--set-vless")
		p.Screen("VLESS config URL")
		uri := "vless://fixture-credential@example.test:443?security=reality#Fixture"
		p.Send("\x1b[200~" + uri + "\x1b[201~")
		p.Screen("•••")
		if strings.Contains(p.Text(), "fixture-credential") {
			t.Fatal("secret in screen")
		}
		path := filepath.Join(home, ".config/xray-codex/servers.json")
		if cancel {
			p.Send("\x1b")
			p.Wait(130)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("cancel wrote servers")
			}
		} else {
			p.Send("\r")
			p.Wait(0)
			b, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(b), "fixture-credential") {
				t.Fatalf("cache %q %v", b, err)
			}
			st, _ := os.Stat(path)
			if st.Mode().Perm() != 0600 {
				t.Fatal(st.Mode())
			}
		}
		if strings.Contains(p.Text(), "fixture-credential") {
			t.Fatal("secret in output")
		}
	}
}
func TestNativeProxyServerSearchCancellationAndFirstRun(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config/xray-codex")
	os.MkdirAll(dir, 0700)
	b, _ := json.Marshal([]object{{"name": "Alpha", "address": "alpha.test", "port": 443}, {"name": "Beta", "address": "beta.test", "port": 443}})
	os.WriteFile(filepath.Join(dir, "servers.json"), b, 0600)
	p := proxyTerminal(t, home, "codex")
	p.Screen("Выбор сервера")
	p.Send("Beta")
	p.Screen("Поиск › Beta")
	p.Send("\x1b")
	p.Wait(130)
	if _, err := os.Stat(filepath.Join(dir, "config.json")); !os.IsNotExist(err) {
		t.Fatal("cancel started proxy")
	}
	p = proxyTerminal(t, t.TempDir(), "codex")
	p.Screen("VLESS subscription URL")
	p.Send("\x1b")
	p.Wait(130)
}
