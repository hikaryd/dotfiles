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
		want += "export " + k + "=\"127.0.0.1,localhost,::1,*.local,private.invalid\"\n"
	}
	if out.String() != want {
		t.Fatalf("output: %q", out.String())
	}
	st, err := os.Stat(filepath.Join(home, ".config", "xray-pi"))
	if err != nil || st.Mode().Perm() != 0700 {
		t.Fatalf("private config directory: %v, %v", st, err)
	}
}
