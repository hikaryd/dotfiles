package desktop

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestRmuxRealTerminalWorkflow(t *testing.T) {
	bin, err := exec.LookPath("rmux")
	if err != nil {
		t.Skip("rmux is not installed")
	}
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	dir, err := os.MkdirTemp(os.TempDir(), "rmx-")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s")
	defer os.RemoveAll(dir)
	environment := envSet(f.env, "PATH="+f.bin+string(os.PathListSeparator)+filepath.Dir(bin)+":/usr/bin:/bin", "TERM=xterm-256color", "LANG=en_US.UTF-8")
	ctl := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, append([]string{"-S", socket}, args...)...)
		cmd.Env = environment
		out, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("rmux %s: %v: %s", strings.Join(args, " "), e, out)
		}
		return strings.TrimSpace(string(out))
	}
	defer func() { _ = exec.Command(bin, "-S", socket, "kill-server").Run() }()
	cwd := f.repo("project with spaces")
	ctl("-f", filepath.Join(root, "config/rmux/rmux.conf"), "new-session", "-d", "-s", "smoke", "-c", cwd, "/bin/sh")
	ctl("set-option", "-g", "default-command", "")
	ctl("set-option", "-g", "default-shell", "/bin/sh")
	ctl("source-file", "-n", filepath.Join(root, "config/rmux/rmux.conf"))
	await := func(label string, check func() bool) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			if check() {
				return
			}
			select {
			case <-deadline:
				t.Fatalf("timeout: %s", label)
			case <-ticker.C:
			}
		}
	}
	client := exec.Command(bin, "-S", socket, "attach-session", "-t", "smoke")
	client.Env = environment
	terminal, err := pty.StartWithSize(client, &pty.Winsize{Rows: 32, Cols: 120})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	defer func() { _ = client.Process.Kill(); _ = client.Wait() }()
	go func() { _, _ = io.Copy(io.Discard, terminal) }()
	await("attached client", func() bool { return ctl("list-clients", "-F", "#{client_name}") != "" })
	keys := func(value string) {
		t.Helper()
		if _, e := terminal.WriteString(value); e != nil {
			t.Fatal(e)
		}
	}
	keys("\x01c")
	await("prefix+c creates a tab", func() bool { return ctl("display-message", "-p", "-t", "smoke", "#{session_windows}") == "2" })
	keys("\x1b[49;5u")
	await("Ctrl+1 selects first tab", func() bool { return ctl("display-message", "-p", "-t", "smoke", "#{window_index}") == "1" })
	keys("\x01|")
	await("prefix+| splits pane", func() bool { return ctl("display-message", "-p", "-t", "smoke", "#{window_panes}") == "2" })
	pane := ctl("display-message", "-p", "-t", "smoke", "#{pane_id}")
	width := ctl("display-message", "-p", "-t", pane, "#{pane_width}")
	keys("\x1b[104;6u")
	await("Ctrl+Shift+h resizes pane", func() bool { return ctl("display-message", "-p", "-t", pane, "#{pane_width}") != width })
	width = ctl("display-message", "-p", "-t", pane, "#{pane_width}")
	keys("\x01Rд")
	await("Russian resize mode", func() bool { return ctl("display-message", "-p", "-t", pane, "#{pane_width}") != width })
	keys("\r\x01с")
	await("Russian prefix+c creates tab", func() bool { return ctl("display-message", "-p", "-t", "smoke", "#{session_windows}") == "3" })
	keys("\x01w")
	await("close only the test tab", func() bool { return ctl("display-message", "-p", "-t", "smoke", "#{session_windows}") == "2" })
	clientID := ctl("list-clients", "-F", "#{client_pid}")
	f.env = envSet(environment, "RMUX="+socket+",42,0", "RMUX_PANE="+pane, "DOTS_MUX_CLIENT="+clientID)
	f.fake("sesh", fakeRule{Args: []string{"list", "-j"}, Output: "[]"})
	out, stderr, code := f.run("", "sessions", "list", "--json")
	if code != 0 || !strings.Contains(out, `"source": "rmux"`) {
		t.Fatalf("live sessions %d: %s %s", code, out, stderr)
	}
	ctl("send-keys", "-t", pane, "printf 'https://example.com/rmux-smoke\\n'", "Enter")
	await("pane output", func() bool {
		return strings.Contains(ctl("capture-pane", "-p", "-t", pane), "https://example.com/rmux-smoke")
	})
	out, stderr, code = f.run("", "refs", "--json")
	if code != 0 || !strings.Contains(out, "https://example.com/rmux-smoke") {
		t.Fatalf("real refs %d: %s %s", code, out, stderr)
	}
	ctl("send-keys", "-t", pane, "printf '\\033]0;Задача rmux\\007'", "Enter")
	defer func() {
		if t.Failed() {
			b, e := os.ReadFile(filepath.Join(f.home, "popup-source"))
			t.Logf("popup source: %q (%v)", b, e)
			t.Logf("pane state: %s", ctl("list-panes", "-a", "-F", "#{pane_id} #{pane_title} #{pane_current_command}"))
			t.Logf("pane output: %q", ctl("capture-pane", "-p", "-t", pane))
		}
	}()
	await("native task title", func() bool { return ctl("display-message", "-p", "-t", pane, "#{pane_title}") == "Задача rmux" })
	// Use the real profile's popup binding, with a bounded test command replacing
	// only the external dots executable at the PATH boundary.
	f.write(filepath.Join(f.bin, "dots"), "#!/bin/sh\nprintf '%s\\n' \"$DOTS_SOURCE_PANE\" > \"$HOME/popup-source\"\n", 0700)
	keys("\x01O")
	await("prefix+O popup ran", func() bool {
		b, _ := os.ReadFile(filepath.Join(f.home, "popup-source"))
		return strings.TrimSpace(string(b)) == pane
	})
	other := f.repo("second project")
	f.fake("sesh", fakeRule{Args: []string{"list", "-j"}, Output: `[{"Src":"tmux","Name":"old tmux","Path":` + strconv.Quote(cwd) + `}]`})
	f.fake("tmux", fakeRule{Args: []string{"list-windows", "-a", "-F", "#{session_name}\t#{window_id}\t#{window_index}\t#{window_name}"}, Output: "old tmux\t@0\t1\teditor\nold tmux\t@1\t2\tshell\n"},
		fakeRule{Args: []string{"list-panes", "-a", "-F", "#{session_name}\t#{window_id}\t#{pane_index}\t#{pane_current_path}"}, Output: "old tmux\t@0\t1\t" + other + "\nold tmux\t@1\t1\t" + cwd + "\nold tmux\t@1\t2\t" + other + "\n"})
	for i := 0; i < 2; i++ {
		out, stderr, code = f.run("", "sessions", "open", "old tmux")
		if code != 0 {
			t.Fatalf("import %d: %d %s %s", i, code, out, stderr)
		}
	}
	if got := strings.Split(ctl("list-sessions", "-F", "#{session_id}"), "\n"); len(got) != 2 {
		t.Fatalf("import not idempotent: %q", got)
	}
	if got := ctl("display-message", "-p", "#{session_windows}"); got != "2" {
		t.Fatalf("import windows=%s", got)
	}
	if got := ctl("display-message", "-p", "#{pane_current_path}"); got != other {
		t.Fatalf("first imported cwd=%q want %q", got, other)
	}
	ctl("switch-client", "-t", "smoke")
	keys("\x01d")
	await("detach retains live sessions", func() bool { return ctl("list-clients", "-F", "#{client_name}") == "" })
	if ctl("display-message", "-p", "-t", "smoke", "#{session_windows}") != "2" {
		t.Fatal("detaching destroyed session")
	}
}
