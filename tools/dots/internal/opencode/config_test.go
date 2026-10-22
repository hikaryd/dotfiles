package opencode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testConfig() Config {
	return Config{APIKey: "test-secret", Port: 6446, ProxyURL: "http://127.0.0.1:10810"}
}
func configPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "private", "config.json")
}
func TestConfigPrivateAtomicAndReload(t *testing.T) {
	p := configPath(t)
	c := testConfig()
	if err := saveConfig(p, c); err != nil {
		t.Fatal(err)
	}
	for path, mode := range map[string]os.FileMode{p: 0600, filepath.Dir(p): 0700} {
		st, err := os.Stat(path)
		if err != nil || st.Mode().Perm() != mode {
			t.Fatalf("mode %s: %v %v", path, st, err)
		}
	}
	c.APIKey = "rotated-secret"
	if err := saveConfig(p, c); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig(p)
	if err != nil || got != c {
		t.Fatalf("reload mismatch: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Fatal("temporary secret left behind")
	}
}
func TestConfigEscapedMaximumKeyRoundTrips(t *testing.T) {
	p := configPath(t)
	c := testConfig()
	c.APIKey = strings.Repeat("<", 8192)
	if err := saveConfig(p, c); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig(p)
	if err != nil || got.APIKey != c.APIKey {
		t.Fatal("accepted escaped key produced unreadable configuration")
	}
}
func TestConfigRefusesIntermediateSymlink(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	target := t.TempDir()
	realPath := filepath.Join(target, "opencode-proxy/config.json")
	if err := saveConfig(realPath, testConfig()); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(base, "dots")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "dots/opencode-proxy/config.json")
	if _, err := loadConfig(path); err == nil {
		t.Error("read secret through intermediate symlink")
	}
	rotated := testConfig()
	rotated.APIKey = "must-not-write"
	if err := saveConfig(path, rotated); err == nil {
		t.Error("wrote secret through intermediate symlink")
	}
	got, err := loadConfig(realPath)
	if err != nil || got.APIKey != "test-secret" {
		t.Fatal("changed foreign secret")
	}
}
func TestConfigAcceptsTrustedBaseSymlink(t *testing.T) {
	base := filepath.Join(t.TempDir(), "config")
	if err := os.Symlink(t.TempDir(), base); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", base)
	path := filepath.Join(base, "dots/opencode-proxy/config.json")
	if err := saveConfig(path, testConfig()); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err != nil {
		t.Fatal(err)
	}
}
func TestConfigRejectsUnsafeInputs(t *testing.T) {
	for _, key := range []string{"", "public", "a b", "a\nb", strings.Repeat("x", 8193)} {
		c := testConfig()
		c.APIKey = key
		if saveConfig(configPath(t), c) == nil {
			t.Fatal("invalid key accepted")
		}
	}
	for _, proxy := range []string{"https://127.0.0.1:10810", "http://example.com:10810", "http://user:pass@127.0.0.1:10810", "http://127.0.0.1:10810/path"} {
		c := testConfig()
		c.ProxyURL = proxy
		if saveConfig(configPath(t), c) == nil {
			t.Fatal("unsafe proxy accepted")
		}
	}
	p := configPath(t)
	if _, err := loadConfig(p); err == nil {
		t.Fatal("missing config accepted")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(p, []byte(`{"api_key":"do-not-leak",`), 0600)
	if _, err := loadConfig(p); err == nil || strings.Contains(err.Error(), "do-not-leak") {
		t.Fatal("bad config error")
	}
}
func TestConfigRefusesSymlinksAndPublicPermissions(t *testing.T) {
	p := configPath(t)
	target := filepath.Join(t.TempDir(), "foreign")
	os.WriteFile(target, []byte("untouched"), 0600)
	os.MkdirAll(filepath.Dir(p), 0700)
	os.Symlink(target, p)
	if saveConfig(p, testConfig()) == nil {
		t.Fatal("followed secret symlink")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "untouched" {
		t.Fatal("changed foreign file")
	}
	os.Remove(p)
	os.Remove(filepath.Dir(p))
	os.Symlink(filepath.Dir(target), filepath.Dir(p))
	if saveConfig(p, testConfig()) == nil {
		t.Fatal("followed directory symlink")
	}
	p = configPath(t)
	saveConfig(p, testConfig())
	os.Chmod(p, 0644)
	if _, err := loadConfig(p); err == nil {
		t.Fatal("public secret file accepted")
	}
}
