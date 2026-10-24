package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Catch installation with the wrong runtime/provider pair, ignored uv failures,
// or a partial mutation when a required executable is missing. uv is the only
// network boundary replaced here; the real installer runs in every case.
func TestAnicliInstall(t *testing.T) {
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		missing  string
		uvExit   string
		wantExit int
		wantHint string
	}{
		{name: "verified versions", uvExit: "0"},
		{name: "uv failure", uvExit: "29", wantExit: 29},
		{name: "missing uv", missing: "uv", wantExit: 1, wantHint: "uv"},
		{name: "missing mpv", missing: "mpv", wantExit: 1, wantHint: "mpv"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			log := filepath.Join(root, "calls")
			for _, name := range []string{"uv", "mpv"} {
				if name == tc.missing {
					continue
				}
				path := filepath.Join(bin, name)
				body := "#!/bin/sh\nexit 0\n"
				if name == "uv" {
					body = "#!/bin/sh\nprintf '%s\\n' \"$@\" >>\"$CALL_LOG\"\nexit \"$UV_EXIT\"\n"
				}
				put(t, path, body)
				if err := os.Chmod(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("/bin/sh", filepath.Join(repo, "scripts/anicli-install.sh"))
			cmd.Env = append(os.Environ(), "PATH="+bin, "CALL_LOG="+log, "UV_EXIT="+tc.uvExit)
			out, err := cmd.CombinedOutput()
			code := 0
			if err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					code = exit.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if code != tc.wantExit || (tc.wantHint != "" && !strings.Contains(string(out), tc.wantHint)) {
				t.Fatalf("exit %d, want %d; output: %s", code, tc.wantExit, out)
			}
			calls, readErr := os.ReadFile(log)
			if tc.missing != "" {
				if !os.IsNotExist(readErr) {
					t.Fatalf("must check all prerequisites before invoking uv: %q, %v", calls, readErr)
				}
				return
			}
			want := "tool\ninstall\n--python\n3.12\n--with\nanicli-api==0.12.0\nanicli-ru==6.1.3\n"
			if readErr != nil || string(calls) != want {
				t.Fatalf("unexpected uv arguments: %q, %v", calls, readErr)
			}
		})
	}
}

// Catch bypassing the shared TUI, splitting user options, or masking the
// client's exit status. Legacy mode remains available without the native binary.
func TestAnicliLauncher(t *testing.T) {
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "shared TUI", true: "legacy"}[legacy], func(t *testing.T) {
			root := t.TempDir()
			name, prefix := "dots-tools", "anime\n"
			args := []string{"--search", "Провожающая в последний путь", "--mpv-opts", "--vo=null --ao=null", ""}
			if legacy {
				name, prefix = "anicli-ru", "cli\n-s\nanilibria\n"
				args = append([]string{"--legacy"}, args...)
			}
			client := filepath.Join(root, name)
			put(t, client, "#!/bin/sh\nprintf '%s\\n' \"$@\" >\"$CALL_LOG\"\nexit 23\n")
			if err := os.Chmod(client, 0700); err != nil {
				t.Fatal(err)
			}
			log := filepath.Join(root, "calls")
			cmd := exec.Command("/bin/sh", append([]string{filepath.Join(repo, "scripts/anicli")}, args...)...)
			cmd.Env = append(os.Environ(), "PATH="+root, "CALL_LOG="+log)
			out, err := cmd.CombinedOutput()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
				t.Fatalf("must preserve exit status: %v, output: %s", err, out)
			}
			want := prefix + "--search\nПровожающая в последний путь\n--mpv-opts\n--vo=null --ao=null\n\n"
			if calls := get(t, log); calls != want {
				t.Fatalf("argument boundaries/default provider lost: %q", calls)
			}
		})
	}
}
