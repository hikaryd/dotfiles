package proxy

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_PROXY_PORT", "11810")
	var out, errOut bytes.Buffer
	if code := run("", []string{"pi", "--print-env"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, &errOut)
	}
	want := ""
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy", "WS_PROXY", "WSS_PROXY", "ws_proxy", "wss_proxy"} {
		want += "export " + k + "=\"http://127.0.0.1:11810\"\n"
	}
	for _, k := range []string{"NO_PROXY", "no_proxy"} {
		want += "export " + k + "=\"127.0.0.1,localhost,::1,*.local\"\n"
	}
	if out.String() != want {
		t.Fatalf("output: %q", out.String())
	}
	st, err := os.Stat(filepath.Join(home, ".config", "xray-pi"))
	if err != nil || st.Mode().Perm() != 0700 {
		t.Fatalf("private config directory: %v, %v", st, err)
	}
}

func TestLocalProxyNameUsesSharedBackendWithoutPi(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DOTS_PROXY_PORT", "11811")
	var out, diagnostic bytes.Buffer
	if code := run("", []string{"local", "--print-env"}, strings.NewReader(""), &out, &diagnostic); code != 0 || !strings.Contains(out.String(), `HTTP_PROXY="http://127.0.0.1:11811"`) {
		t.Fatalf("shared proxy: %d %q %q", code, out.String(), diagnostic.String())
	}
}

func TestProxyCannotLaunchRemovedPiAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{"local", "pi"} {
		var out, diagnostic bytes.Buffer
		if code := run("", []string{name, "--run", "--", "-p", "do not launch"}, strings.NewReader(""), &out, &diagnostic); code != 2 {
			t.Fatalf("removed agent launch through %s returned %d: %s", name, code, &diagnostic)
		}
	}
}
