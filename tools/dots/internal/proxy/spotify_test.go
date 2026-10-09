package proxy

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSpotifyPrefs(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "Library", "Application Support", "Spotify", "prefs")
	var out, errs bytes.Buffer
	invoke := func(args ...string) int { return runSpotify(home, args, &out, &errs) }
	if code := invoke("--check"); code != 1 {
		t.Fatalf("missing check: %d", code)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("check created parent: %v", err)
	}
	original := "other.setting=true\n# preserve comment\nnetwork.proxy.mode=1\nnetwork.proxy.mode=2\nnetwork.proxy.addr=\"old\"\n\nlast=value\n"
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(original), 0640); err != nil {
		t.Fatal(err)
	}
	// chmod establishes the preservation fixture regardless of the runner's
	// private umask; WriteFile's creation mode alone is masked by the process.
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if code := invoke("--check"); code != 1 {
		t.Fatalf("duplicate check: %d", code)
	}
	if code := invoke("--port", "11810"); code != 0 {
		t.Fatalf("write: %d %s", code, &errs)
	}
	got, _ := os.ReadFile(path)
	want := "other.setting=true\n# preserve comment\n\nlast=value\nnetwork.proxy.mode=2\nnetwork.proxy.addr=\"127.0.0.1:11810@http\"\n"
	if string(got) != want {
		t.Fatalf("prefs %q", got)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0640 {
		t.Fatal(st.Mode())
	}
	old := time.Unix(1000000000, 0)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if code := invoke("--port=11810", "--check"); code != 0 {
		t.Fatal(code)
	}
	if code := invoke("--port=11810"); code != 0 {
		t.Fatal(code)
	}
	st, _ = os.Stat(path)
	if !st.ModTime().Equal(old) {
		t.Fatal("idempotent call rewrote preferences")
	}
	for _, args := range [][]string{{"--port", "0"}, {"--port", "65536"}, {"--port", "bad"}, {"--unknown"}, {"extra"}} {
		if invoke(args...) != 2 {
			t.Fatalf("bad args accepted: %v", args)
		}
	}
}
func TestSpotifyUTF8FailureDoesNotLeak(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "Library", "Application Support", "Spotify", "prefs")
	if err := atomicWrite(path, []byte("secret-fixture\xff"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if code := runSpotify(home, nil, &out, &errs); code != 2 || strings.Contains(errs.String(), "secret-fixture") {
		t.Fatalf("%d %s", code, &errs)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "secret-fixture\xff" {
		t.Fatal("invalid preferences modified")
	}
}
