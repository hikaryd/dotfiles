package integration_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"dots/native/tests/testutil/testpty"
)

func TestRmuxExtendedDeletionKeys(t *testing.T) {
	bin, err := exec.LookPath("rmux")
	if err != nil {
		t.Skip("rmux is not installed")
	}
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	dir, err := os.MkdirTemp(os.TempDir(), "rmx-keys-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "s")
	environment := envSet(f.env, "PATH=/usr/bin:/bin", "TERM=xterm-ghostty", "TERM_PROGRAM=ghostty", "LANG=en_US.UTF-8")
	ctl := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, append([]string{"-S", socket}, args...)...)
		cmd.Env = environment
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("rmux %v: %v: %s", args, err, out)
		}
		return string(out)
	}
	defer func() { _ = exec.Command(bin, "-S", socket, "kill-server").Run() }()
	output := filepath.Join(f.home, "input")
	// Jcode requests modifyOtherKeys mode 2 when Kitty negotiation is unavailable.
	command := "/bin/stty raw -echo; printf '\\033[>4;2mKEYS_READY'; exec /bin/cat > " + strconv.Quote(output)
	ctl("-f", filepath.Join(root, "config/rmux/rmux.conf"), "new-session", "-d", "-s", "keys", command)
	client := exec.Command(bin, "-S", socket, "attach-session", "-t", "keys")
	client.Env = environment
	terminal := testpty.Start(t, client)
	terminal.Screen("KEYS_READY")

	for _, tc := range []struct {
		name, input, want string
	}{
		{"option_backspace", "\x1b\x7f", "\x1b\x7f"},
		{"option_backspace_csi_u", "\x1b[127;3u", "\x1b\x7f"},
		{"command_backspace", "\x15", "\x1b[117;5u"},
		{"control_u_csi_u", "\x1b[117;5u", "\x1b[117;5u"},
		{"backspace", "\x7f", "\x7f"},
		{"shift_enter", "\x1b[13;2u", "\x1b[13;2u"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			terminal.Send(tc.input + "!")
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				after, err := os.ReadFile(output)
				if err != nil {
					t.Fatal(err)
				}
				got := string(after[len(before):])
				if strings.HasSuffix(got, "!") {
					if got != tc.want+"!" {
						t.Fatalf("pane received %q, want %q", got, tc.want+"!")
					}
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("pane did not receive the key and marker")
		})
	}
}
