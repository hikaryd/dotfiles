package vpn

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCLIStatusesAndRedaction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("VPN_ENV", filepath.Join(t.TempDir(), "missing.env"))
	for _, c := range []struct {
		args []string
		code int
	}{{nil, 2}, {[]string{"--help"}, 0}, {[]string{"generate"}, 1}, {[]string{"unknown-secret"}, 2}, {[]string{"benchmark", "--help"}, 0}, {[]string{"benchmark", "--samples", "0"}, 2}, {[]string{"benchmark", "--all", "--only", "fake-secret"}, 2}, {[]string{"benchmark", "--url", "https://fake-secret@host"}, 2}, {[]string{"benchmark", "--max-duration", "NaN"}, 2}, {[]string{"xhttp-benchmark", "--help"}, 0}, {[]string{"xhttp-benchmark", "--timeout", "NaN"}, 2}, {[]string{"xhttp-benchmark", "--check-only", "--install-only"}, 2}, {[]string{"tcp-tuning", "--help"}, 0}, {[]string{"tcp-tuning", "apply", "--peer", "10.8.2.2/32"}, 2}} {
		var out, errout bytes.Buffer
		equal(t, run(c.args, &out, &errout), c.code)
		if strings.Contains(out.String()+errout.String(), "fake-secret") || strings.Contains(out.String()+errout.String(), "unknown-secret") {
			t.Fatal("unsafe CLI diagnostics")
		}
	}
	t.Setenv("ACTION", "bad")
	var out, errout bytes.Buffer
	equal(t, run([]string{"router-setup"}, &out, &errout), 2)
}
func TestBenchmarkCLISignalCleansOwnedChild(t *testing.T) {
	requireSocket(t)
	dir := fakeCommands(t)
	t.Setenv("VPN_TEST_CURL_SLEEP", "10s")
	self, _ := os.Executable()
	binary := filepath.Join(dir, "dots-vpn")
	if e := os.Symlink(self, binary); e != nil {
		t.Fatal(e)
	}
	cfg := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(cfg, jsonBytes(object{"outbounds": []object{{"type": "vless", "tag": "manual", "uuid": "fixture-uuid"}}}), 0600)
	cmd := exec.Command(binary, "benchmark", "--config", cfg, "--only", "manual", "--samples", "1", "--bytes", "1000")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { cmd.Process.Kill() }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		found := false
		for _, r := range traces(t) {
			if r["command"] == "curl" {
				found = true
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("curl did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if e := cmd.Process.Signal(syscall.SIGTERM); e != nil {
		t.Fatal(e)
	}
	select {
	case <-done:
	case <-time.After(7 * time.Second):
		t.Fatal("signal cleanup timeout")
	}
	equal(t, cmd.ProcessState.ExitCode(), 130)
	assertCleanChildren(t)
	if !strings.Contains(stderr.String(), "cancelled") {
		t.Fatal(stderr.String())
	}
}
func TestURLQueryTokenNotInProcessArgv(t *testing.T) {
	fakeCommands(t)
	source := "https://fixture.example/file?token=fake-query-secret&bytes={bytes}"
	r := measurement(context.Background(), 1234, "fake-password", 1000, time.Second, source)
	equal(t, r["ok"], true)
	for _, row := range traces(t) {
		if strings.Contains(string(jsonBytes(row["args"])), "fake-query-secret") {
			t.Fatal("query credential in argv")
		}
		if !strings.Contains(str(row["stdin"]), "fake-query-secret") {
			t.Fatal("source missing in stdin")
		}
	}
}

func TestNativeCLIProcessArgvExit(t *testing.T) {
	dir := fakeCommands(t)
	self, _ := os.Executable()
	binary := filepath.Join(dir, "dots-vpn")
	if e := os.Symlink(self, binary); e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		args []string
		code int
	}{{[]string{"--help"}, 0}, {[]string{"benchmark", "--samples", "0"}, 2}, {[]string{"xhttp-benchmark", "--source-index", "-1"}, 2}} {
		cmd := exec.Command(binary, c.args...)
		var out, errout bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errout
		_ = cmd.Run()
		equal(t, cmd.ProcessState.ExitCode(), c.code)
		if c.code == 0 && !strings.Contains(out.String(), "dots-vpn") {
			t.Fatal(out.String())
		}
		if c.code != 0 && errout.Len() == 0 {
			t.Fatal("missing CLI failure diagnostic")
		}
	}
}
