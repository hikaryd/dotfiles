package proxy

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCodeDispatchDoesNotInitializeXray(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_PROXY_PORT", "invalid-xray-setting")
	var out, errOut bytes.Buffer
	if code := run("", []string{"opencode", "--help"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("opencode dispatch: %d %s", code, &errOut)
	}
	if !strings.Contains(out.String(), "setup [--stdin]") {
		t.Fatal("missing opencode CLI")
	}
	if _, err := os.Stat(filepath.Join(home, ".config/xray-pi")); !os.IsNotExist(err) {
		t.Fatal("opencode touched xray state")
	}
}
