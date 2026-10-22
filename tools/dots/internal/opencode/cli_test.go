package opencode

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testCLI(t *testing.T) *cli {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PI_PROXY_PORT", "10810")
	return &cli{ctx: context.Background(), home: home, config: filepath.Join(home, ".config/dots/opencode-proxy/config.json"), base: filepath.Join(home, ".config"), binary: filepath.Join(home, "installed/dots-tools"), in: strings.NewReader(""), out: &bytes.Buffer{}, errOut: &bytes.Buffer{}}
}
func fakeCommand(t *testing.T, name, body string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bin")
	os.Mkdir(dir, 0700)
	os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0700)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}
func TestCLIKeyCanConfigureWithoutInstalling(t *testing.T) {
	c := testCLI(t)
	if code := Run([]string{"key", "--stdin"}, strings.NewReader("first-private-key\n"), c.out, c.errOut); code != 0 {
		t.Fatalf("first key configuration failed: %v", c.errOut)
	}
	got, err := loadConfig(c.config)
	if err != nil || got.APIKey != "first-private-key" {
		t.Fatal("initial key not saved")
	}
	for _, path := range []string{c.servicePath(), filepath.Join(c.base, "jevgrep")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("key command installed service or changed jg")
		}
	}
	if strings.Contains(fmt.Sprint(c.out, c.errOut), "first-private-key") {
		t.Fatal("key leaked in output")
	}
}
func TestCLIExplicitStdinAndSecretRotation(t *testing.T) {
	c := testCLI(t)
	saveConfig(c.config, testConfig())
	var out, errOut bytes.Buffer
	code := Run([]string{"key"}, strings.NewReader("new-secret\n"), &out, &errOut)
	if code == 0 || !strings.Contains(errOut.String(), "--stdin") {
		t.Fatalf("implicit stdin accepted: %d %s", code, &errOut)
	}
	out.Reset()
	errOut.Reset()
	code = Run([]string{"key", "--stdin"}, strings.NewReader("new-secret\n"), &out, &errOut)
	if code != 0 {
		t.Fatalf("rotation failed: %s", &errOut)
	}
	got, err := loadConfig(c.config)
	if err != nil || got.APIKey != "new-secret" {
		t.Fatal("key did not rotate")
	}
	if strings.Contains(out.String()+errOut.String(), "new-secret") {
		t.Fatal("key leaked")
	}
	for _, args := range [][]string{{"key", "--key", "secret"}, {"status", "--stdin"}, {"serve", "--config"}, {"unknown"}} {
		if Run(args, strings.NewReader(""), &out, &errOut) == 0 {
			t.Fatalf("invalid arguments accepted %v", args)
		}
	}
}

type observedLockContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *observedLockContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}
func TestSetupPreservesConcurrentKeyRotation(t *testing.T) {
	c := testCLI(t)
	if err := saveConfig(c.config, testConfig()); err != nil {
		t.Fatal(err)
	}
	fakeCommand(t, "jg", "exit 1\n")
	unlock, err := c.lock()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	observed := &observedLockContext{Context: context.Background(), waiting: make(chan struct{})}
	c.ctx = observed
	ended := make(chan error, 1)
	go func() { ended <- c.execute("setup", false) }()
	select {
	case <-observed.waiting:
	case <-time.After(time.Second):
		t.Fatal("setup did not wait for configuration lock")
	}
	rotated := testConfig()
	rotated.APIKey = "rotated-while-setup-waited"
	if err := saveConfig(c.config, rotated); err != nil {
		t.Fatal(err)
	}
	unlock()
	unlock = nil
	select {
	case err := <-ended:
		// The test deliberately has no installed binary, so no service is touched.
		if err == nil {
			t.Fatal("unexpected service installation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("setup did not release configuration lock")
	}
	got, err := loadConfig(c.config)
	if err != nil || got.APIKey != rotated.APIKey {
		t.Fatal("repeat setup restored a stale key after rotation")
	}
}
func TestCLIStatusIsReadOnly(t *testing.T) {
	c := testCLI(t)
	fakeCommand(t, "launchctl", "exit 113\n")
	if code := Run([]string{"status"}, strings.NewReader(""), c.out, c.errOut); code == 0 {
		t.Fatal("unconfigured status success")
	}
	if _, err := os.Stat(filepath.Join(c.base, "dots")); !os.IsNotExist(err) {
		t.Fatal("status created state")
	}
	if code := Run([]string{"--help"}, strings.NewReader(""), c.out, c.errOut); code != 0 {
		t.Fatal("help failed")
	}
}
func TestJevgrepBackupAndRollback(t *testing.T) {
	c := testCLI(t)
	saveConfig(c.config, testConfig())
	p := filepath.Join(c.base, "jevgrep/credentials.json")
	os.MkdirAll(filepath.Dir(p), 0700)
	old := []byte(`{"provider":"custom","apiKey":"prior-private-key","baseURL":"https://example.com/v1","model":"old"}`)
	os.WriteFile(p, old, 0600)
	t.Setenv("JG_CREDS", p)
	t.Setenv("JG_ARGS", filepath.Join(c.home, "args"))
	fakeCommand(t, "jg", `printf '%s\n' "$@" > "$JG_ARGS"
read key
[ "$key" = local ] || exit 3
printf '%s' '{"provider":"custom","apiKey":"local","baseURL":"http://127.0.0.1:6446/v1","model":"jev-1.13-free"}' > "$JG_CREDS"
`)
	if err := c.connectJevgrep(testConfig()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(c.backupPath())
	if err != nil || !bytes.Equal(b, old) {
		t.Fatal("backup missing or changed")
	}
	st, _ := os.Stat(c.backupPath())
	if st.Mode().Perm() != 0600 {
		t.Fatal("backup not private")
	}
	args, _ := os.ReadFile(os.Getenv("JG_ARGS"))
	if !strings.Contains(string(args), "http://127.0.0.1:6446/v1") || strings.Contains(string(args), "test-secret") {
		t.Fatal("wrong auth args")
	}
	os.Remove(os.Getenv("JG_ARGS"))
	if err := c.connectJevgrep(testConfig()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(os.Getenv("JG_ARGS")); !os.IsNotExist(err) {
		t.Fatal("idempotent setup invoked auth")
	}
	os.WriteFile(p, old, 0600)
	fakeCommand(t, "jg", `printf broken > "$JG_CREDS"
exit 1
`)
	if err := c.connectJevgrep(testConfig()); err == nil || strings.Contains(err.Error(), "private-key") {
		t.Fatal("unsafe auth failure")
	}
	got, _ := os.ReadFile(p)
	if !bytes.Equal(got, old) {
		t.Fatal("failed auth destroyed credentials")
	}
}
