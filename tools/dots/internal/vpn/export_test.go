package vpn

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportFreshNodesWithoutGatewayChanges(t *testing.T) {
	dir := t.TempDir()
	s := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, vlessURI+"\n"+hyURI+"\nunsupported://fixture\n")
	}))
	conf, destination := filepath.Join(dir, "vpn.env"), filepath.Join(dir, "benchmark.json")
	os.WriteFile(conf, []byte("SUB_URL="+s.URL+"?token=PRIVATE\n"), 0600)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("gateway unchanged"), 0600)
	var out, diagnostic bytes.Buffer
	if code := run([]string{"nodes", "--env", conf, "--output", destination}, &out, &diagnostic); code != 0 {
		t.Fatal(code, diagnostic.String())
	}
	if out.String() != "Строк: 3; разобрано: 2; пропущено: 1\n" || strings.Contains(out.String()+diagnostic.String(), "PRIVATE") {
		t.Fatal(out.String(), diagnostic.String())
	}
	info, err := os.Stat(destination)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	data, _ := os.ReadFile(destination)
	var cfg struct {
		Outbounds []object `json:"outbounds"`
	}
	if json.Unmarshal(data, &cfg) != nil || len(cfg.Outbounds) != 2 {
		t.Fatal(string(data))
	}
	gateway, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	if string(gateway) != "gateway unchanged" {
		t.Fatal("gateway changed")
	}
}

func TestExportRejectsSymlinkAndMissingArguments(t *testing.T) {
	dir := t.TempDir()
	target, link := filepath.Join(dir, "target"), filepath.Join(dir, "link")
	os.WriteFile(target, []byte("preserve"), 0600)
	os.Symlink(target, link)
	for _, c := range []struct {
		args []string
		code int
	}{
		{[]string{"nodes", "--help"}, 0}, {[]string{"nodes"}, 2}, {[]string{"nodes", "--output", link}, 1},
		{[]string{"nodes", "--output", filepath.Join(dir, "new"), "--env", filepath.Join(dir, "absent")}, 1},
	} {
		var out, diagnostic bytes.Buffer
		if code := run(c.args, &out, &diagnostic); code != c.code {
			t.Fatalf("%v: %d", c.args, code)
		}
	}
	data, _ := os.ReadFile(target)
	if string(data) != "preserve" {
		t.Fatal("target overwritten")
	}
	info, _ := os.Lstat(link)
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink overwritten")
	}
}
