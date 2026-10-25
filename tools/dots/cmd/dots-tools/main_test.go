package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureDispatch(t *testing.T, root, name string, args []string) (int, string, string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	code := dispatch(root, name, args)
	outW.Close()
	errW.Close()
	out, _ := io.ReadAll(outR)
	diagnostic, _ := io.ReadAll(errR)
	outR.Close()
	errR.Close()
	return code, string(out), string(diagnostic)
}

func TestNativeAliasesAndSetupHelpDoNotApply(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	for _, command := range []string{"pi-install", "pi-apply", "jevgrep-apply", "waterfox-apply", "tinycast-hotkey", "managed-links", "json-keys"} {
		code, out, err := captureDispatch(t, root, "dots-tools", []string{"setup", command, "--help"})
		if code != 0 || out == "" || err != "" {
			t.Fatalf("help %s: %d %q %q", command, code, out, err)
		}
	}
	for _, name := range []string{"pi-install", "pi-install.sh"} {
		if code, _, _ := captureDispatch(t, root, name, []string{"--help"}); code != 0 {
			t.Fatalf("alias %s: %d", name, code)
		}
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("help changed HOME: %v %v", entries, err)
	}
}

func TestVPNInstallerHelpersDispatchThroughBothEntrypoints(t *testing.T) {
	for _, alias := range []bool{false, true} {
		root := t.TempDir()
		name := "dots-tools"
		prefix := []string{"vpn"}
		if alias {
			name, prefix = "dots-vpn", nil
		}
		path := filepath.Join(root, "awg.env")
		code, out, diagnostic := captureDispatch(t, root, name, append(prefix, "awg-params", path))
		if code != 0 || out != "" || diagnostic != "" {
			t.Fatalf("AWG dispatch: %d %q %q", code, out, diagnostic)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("private AWG file: %v %v", info, err)
		}
		conf := filepath.Join(root, "vpn.env")
		if err := os.WriteFile(conf, []byte("SUB_URL=PRIVATE\nAUTO_INCLUDE=\nAUTO_PREFER=Hysteria\n"), 0600); err != nil {
			t.Fatal(err)
		}
		code, out, diagnostic = captureDispatch(t, root, name, append(prefix, "tuning-env", conf))
		if code != 0 || out != "HEALTH_PORT=7896\nAUTO_INCLUDE=\nAUTO_PREFER=Hysteria\n" || diagnostic != "" {
			t.Fatalf("tuning dispatch: %d %q %q", code, out, diagnostic)
		}
	}
}

func TestStandaloneClusterToolIsNotDispatchedByDots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	for _, entry := range []struct {
		name string
		args []string
	}{
		{"dots-tools", []string{"kube", "commands"}},
		{"dots-kube", []string{"commands"}},
	} {
		code, out, diagnostic := captureDispatch(t, "", entry.name, entry.args)
		if code != 2 || out != "" || !strings.Contains(diagnostic, "unknown group") {
			t.Fatalf("retired entrypoint %s %v: %d %q %q", entry.name, entry.args, code, out, diagnostic)
		}
	}
	code, out, diagnostic := captureDispatch(t, "", "dots-tools", []string{"--help"})
	if code != 0 || diagnostic != "" || strings.Contains(out, "kube") {
		t.Fatalf("help advertises standalone tooling: %d %q %q", code, out, diagnostic)
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("retired commands changed HOME: %v %v", entries, err)
	}
}

func TestDesktopAliasHasSafeReadablePreview(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	code, out, diagnostic := captureDispatch(t, t.TempDir(), "dots", []string{"menu", "--preview", "--plain"})
	if code != 0 || diagnostic != "" || !strings.Contains(out, "Работа") || strings.Contains(out, "\x1b") {
		t.Fatalf("preview: %d %q %q", code, out, diagnostic)
	}
	if code, _, _ := captureDispatch(t, "", "dots-tools", []string{"unknown"}); code != 2 {
		t.Fatal("unknown group accepted")
	}
}

func TestAnimeHelpDoesNotRequireTerminalOrInstalledRuntime(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	code, out, diagnostic := captureDispatch(t, "", "dots-tools", []string{"anime", "--help"})
	if code != 0 || !strings.Contains(out, "dots anime") || !strings.Contains(out, "--legacy") || diagnostic != "" {
		t.Fatalf("anime help: %d %q %q", code, out, diagnostic)
	}
	code, _, diagnostic = captureDispatch(t, "", "dots-tools", []string{"anime", "--unknown"})
	if code != 2 || !strings.Contains(diagnostic, "unknown") {
		t.Fatalf("anime invalid arguments: %d %q", code, diagnostic)
	}
}
