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
	for _, command := range []string{"pi-install", "pi-apply", "jevgrep-apply", "codex-instructions-apply", "codex-task-title-apply", "waterfox-apply", "tinycast-hotkey", "claude-apply-json", "claude-sync-json", "codexbar-config", "managed-links", "json-keys"} {
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

func TestKubernetesAliasesAndGroupDispatchWithoutLoadingConfiguration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{"kpdev", "kpstage", "kppreprod", "kpprod", "kstage", "kprod-connect", "kpdev-logs", "kpstage-logs-save", "kppreprod-jobs", "kpprod-jobs-save", "kpdev-cron-run", "kpstage-exec", "kpprod-log-search", "kpdev-logs-multi", "kpstage-scale", "kppreprod-pod-restart", "kpprod-pod-analyze", "kpstage-deploy-watch", "kube-tools-help", "kube-log-watch", "kube-deploy-watch", "kube-pod-analyze"} {
		code, out, diagnostic := captureDispatch(t, "", name, []string{"--help"})
		if code != 0 || out+diagnostic == "" {
			t.Fatalf("alias %s: %d %q %q", name, code, out, diagnostic)
		}
	}
	code, out, diagnostic := captureDispatch(t, "", "dots-tools", []string{"kube", "kpstage", "--help"})
	if code != 0 || !strings.Contains(out, "namespace") || diagnostic != "" {
		t.Fatalf("group %d %q %q", code, out, diagnostic)
	}
	code, out, diagnostic = captureDispatch(t, "", "dots-tools", []string{"kube", "commands"})
	if code != 0 || !strings.Contains(out, "kpstage\n") || !strings.Contains(out, "kpprod-pod-restart\n") || diagnostic != "" {
		t.Fatalf("commands %d %q %q", code, out, diagnostic)
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
