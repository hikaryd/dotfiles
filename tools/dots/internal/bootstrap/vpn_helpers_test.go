package bootstrap_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"dots/native/internal/bootstrap"
)

func runVPNHelper(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	dir := t.TempDir()
	out, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	errout, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		out.Close()
		t.Fatal(err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = out, errout
	defer func() {
		os.Stdout, os.Stderr = stdout, stderr
		out.Close()
		errout.Close()
	}()
	status := bootstrap.RunVPNHelper("unused-root", args)
	read := func(path string) string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	return status, read(out.Name()), read(errout.Name())
}

func writeVPNFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestVPNHelperAWGParameters(t *testing.T) {
	dir := t.TempDir()
	for sample := 0; sample < 64; sample++ {
		path := filepath.Join(dir, fmt.Sprintf("params-%d.env", sample))
		status, out, errout := runVPNHelper(t, "awg-params", path)
		if status != 0 || out != "" || errout != "" {
			t.Fatalf("creation: exit=%d stdout=%q stderr=%q", status, out, errout)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("expected private file: %v, %v", info, err)
		}
		values := map[string]int{}
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			key, value, ok := strings.Cut(line, "=")
			n, err := strconv.Atoi(value)
			if !ok || err != nil {
				t.Fatalf("invalid parameter line %q", line)
			}
			if _, exists := values[key]; exists {
				t.Fatalf("duplicate key %q", key)
			}
			values[key] = n
		}
		if len(values) != 9 {
			t.Fatalf("wrong parameter count: %v", values)
		}
		bounds := map[string][2]int{
			"Jc": {3, 8}, "Jmin": {8, 60}, "Jmax": {values["Jmin"] + 10, 120},
			"S1": {15, 150}, "S2": {15, 150},
			"H1": {5, 2147483647}, "H2": {5, 2147483647},
			"H3": {5, 2147483647}, "H4": {5, 2147483647},
		}
		seen := map[int]bool{}
		for key, bounds := range bounds {
			n := values[key]
			if n < bounds[0] || n > bounds[1] {
				t.Fatalf("%s=%d outside %v", key, n, bounds)
			}
			if strings.HasPrefix(key, "H") {
				if seen[n] {
					t.Fatalf("duplicate H: %v", values)
				}
				seen[n] = true
			}
		}
		if values["S1"]+56 == values["S2"] {
			t.Fatalf("forbidden S combination: %v", values)
		}
		status, out, errout = runVPNHelper(t, "awg-params", path)
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(info, after) || info.ModTime() != after.ModTime() {
			t.Fatalf("reinstall replaced or touched parameters: %v", err)
		}
		again, err := os.ReadFile(path)
		if err != nil || string(again) != string(data) || status != 0 || out != "" || errout != "" {
			t.Fatalf("reinstall changed parameters: exit=%d stdout=%q stderr=%q error=%v", status, out, errout, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 64 {
		t.Fatalf("temporary files left behind: %v, %v", entries, err)
	}
}

func TestVPNHelperAWGPreservesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.env")
	writeVPNFixture(t, path, "user-owned settings\n")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	status, out, errout := runVPNHelper(t, "awg-params", path)
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || after.Mode() != before.Mode() || before.ModTime() != after.ModTime() {
		t.Fatalf("existing file metadata changed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "user-owned settings\n" || status != 0 || out != "" || errout != "" {
		t.Fatalf("existing file changed: exit=%d stdout=%q stderr=%q error=%v", status, out, errout, err)
	}
}

func TestVPNHelperAWGRejectsUnsafeDestinations(t *testing.T) {
	for _, kind := range []string{"symlink", "dangling-symlink", "directory", "missing-parent"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "params")
			target := filepath.Join(dir, "target")
			switch kind {
			case "symlink", "dangling-symlink":
				if kind == "symlink" {
					writeVPNFixture(t, target, "preserve me")
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			case "missing-parent":
				path = filepath.Join(dir, "missing", "params")
			}
			before, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			status, out, errout := runVPNHelper(t, "awg-params", path)
			if status != 1 || out != "" || errout == "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q", status, out, errout)
			}
			after, err := os.ReadDir(dir)
			if err != nil || len(before) != len(after) {
				t.Fatalf("failure left partial files: %v", err)
			}
			if strings.Contains(kind, "symlink") {
				if got, err := os.Readlink(path); err != nil || got != target {
					t.Fatalf("symlink replaced: %q, %v", got, err)
				}
				if kind == "symlink" {
					if data, err := os.ReadFile(target); err != nil || string(data) != "preserve me" {
						t.Fatalf("symlink target changed: %v", err)
					}
				} else if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("dangling target created: %v", err)
				}
			}
		})
	}
}

func TestVPNHelperAWGConcurrentPublish(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params")
	var wg sync.WaitGroup
	statuses := make(chan int, 16)
	for range 16 {
		wg.Go(func() { statuses <- bootstrap.RunVPNHelper("", []string{"awg-params", path}) })
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != 0 {
			t.Fatalf("concurrent writer exited %d", status)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Count(string(data), "\n") != 9 {
		t.Fatalf("incomplete publication: %q, %v", data, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v, %v", entries, err)
	}
}

func TestVPNHelperTuningParsingAndAllowlist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vpn.env")
	marker := filepath.Join(dir, "must-not-exist")
	writeVPNFixture(t, path, " # comment\r\ninvalid line\r\n"+
		"SUB_URL=https://example.invalid/private-token\nPASSWORD=private-password\n"+
		"export AUTO_MAX=99\nHEALTH_PORT = \" +07897 \"\nAUTO_INCLUDE=old\n"+
		"AUTO_INCLUDE = \"'DE=fast|NL'\"\nAUTO_EXCLUDE=''\nAUTO_MAX=\n"+
		"AUTO_PREFER=$(touch "+marker+") # literal\rUNRELATED=secret\r")
	status, out, errout := runVPNHelper(t, "tuning-env", path)
	want := "HEALTH_PORT=7897\nAUTO_INCLUDE=DE=fast|NL\nAUTO_EXCLUDE=\nAUTO_MAX=\nAUTO_PREFER=$(touch " + marker + ") # literal\n"
	if status != 0 || out != want || errout != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q; want %q", status, out, errout, want)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("shell expression executed: %v", err)
	}
}

func TestVPNHelperTuningDefaultsAndPathPrecedence(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, "env")
	t.Setenv("VPN_ENV", envPath)
	t.Setenv("HEALTH_PORT", "9999")
	t.Setenv("AUTO_INCLUDE", "must-not-inherit")
	for _, content := range []string{"", "# nothing\n", "AUTO_EXCLUDE=\n"} {
		writeVPNFixture(t, envPath, content)
		status, out, errout := runVPNHelper(t, "tuning-env")
		want := "HEALTH_PORT=7896\n"
		if strings.Contains(content, "AUTO_EXCLUDE") {
			want += "AUTO_EXCLUDE=\n"
		}
		if status != 0 || out != want || errout != "" {
			t.Fatalf("defaults: exit=%d stdout=%q stderr=%q", status, out, errout)
		}
	}
	writeVPNFixture(t, envPath, "HEALTH_PORT=9000\n")
	status, out, errout := runVPNHelper(t, "tuning-env", filepath.Join(dir, "absent"))
	if status != 0 || out != "HEALTH_PORT=7896\n" || errout != "" {
		t.Fatalf("explicit missing path: exit=%d stdout=%q stderr=%q", status, out, errout)
	}
	status, out, errout = runVPNHelper(t, "tuning-env")
	if status != 0 || out != "HEALTH_PORT=9000\n" || errout != "" {
		t.Fatalf("VPN_ENV path: exit=%d stdout=%q stderr=%q", status, out, errout)
	}
}

func TestVPNHelperTuningHealthPort(t *testing.T) {
	for _, port := range []string{"1", "65535", "+7896", "0007896", "0", "65536", "-1", "", "private-password", "7896 # comment", "1.5", "99999999999999999999999"} {
		t.Run(port, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "vpn.env")
			writeVPNFixture(t, path, "AUTO_INCLUDE=private-filter\nSUB_URL=private-url\nHEALTH_PORT="+port+"\n")
			status, out, errout := runVPNHelper(t, "tuning-env", path)
			n, err := strconv.Atoi(port)
			if err == nil && n >= 1 && n <= 65535 {
				want := fmt.Sprintf("HEALTH_PORT=%d\nAUTO_INCLUDE=private-filter\n", n)
				if status != 0 || out != want || errout != "" {
					t.Fatalf("exit=%d stdout=%q stderr=%q", status, out, errout)
				}
			} else if status != 1 || out != "" || errout != "HEALTH_PORT must be in 1..65535\n" {
				t.Fatalf("invalid port must fail before output: exit=%d stdout=%q stderr=%q", status, out, errout)
			}
		})
	}
}

func TestVPNHelperTuningReadFailures(t *testing.T) {
	dir := t.TempDir()
	invalidUTF8 := filepath.Join(dir, "invalid")
	writeVPNFixture(t, invalidUTF8, "SUB_URL=private-url\nAUTO_INCLUDE=\xff\n")
	for _, path := range []string{dir, invalidUTF8} {
		status, out, errout := runVPNHelper(t, "tuning-env", path)
		if status != 1 || out != "" || errout == "" || strings.Contains(errout, path) || strings.Contains(errout, "private-url") {
			t.Fatalf("read failure: exit=%d stdout=%q stderr=%q", status, out, errout)
		}
	}
}

func TestVPNHelperUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown-secret"}, {"awg-params"}, {"awg-params", ""}, {"awg-params", "a", "b"}, {"tuning-env", "a", "b"}} {
		status, out, errout := runVPNHelper(t, args...)
		if status != 2 || out != "" || !strings.HasPrefix(errout, "usage:") || strings.Contains(errout, "unknown-secret") {
			t.Fatalf("usage: exit=%d stdout=%q stderr=%q", status, out, errout)
		}
	}
}
