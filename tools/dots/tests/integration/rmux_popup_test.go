package integration_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"dots/native/tests/testutil/testpty"
)

func TestRmuxPopupSurvivesUnderlayRefresh(t *testing.T) {
	bin := os.Getenv("DOTS_TEST_RMUX_POPUP_BIN")
	if bin == "" {
		t.Skip("set DOTS_TEST_RMUX_POPUP_BIN to test a candidate rmux/daemon pair")
	}
	var err error
	bin, err = filepath.Abs(bin)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	dir, err := os.MkdirTemp(os.TempDir(), "rmx-popup-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "s")
	environment := envSet(f.env, "PATH=/usr/bin:/bin", "TERM=xterm-ghostty", "TERM_PROGRAM=ghostty")
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
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "-S", socket, "kill-server")
		cmd.Env = environment
		_ = cmd.Run()
	}()
	underlay := `/bin/sh -c 'n=0; while :; do printf "\033[1;1HMAIN_SENTINEL_%s" "$n"; n=$((n+1)); sleep 0.02; done'`
	ctl("-f", filepath.Join(root, "config/rmux/rmux.conf"), "new-session", "-d", "-s", "popup", underlay)
	client := exec.Command(bin, "-S", socket, "attach-session", "-t", "popup")
	client.Env = environment
	terminal := testpty.Start(t, client)
	terminal.Screen("MAIN_SENTINEL")
	ctl("display-popup", "-E", "-w", "75%", "-h", "70%", `/bin/sh -c 'printf POPUP_SENTINEL; exec sleep 30'`)
	terminal.Screen("POPUP_SENTINEL")
	frames := regexp.MustCompile("(?s)\\x1b\\[\\?2026h(.*?)\\x1b\\[\\?2026l")
	checkPopup := func(offset int) {
		t.Helper()
		time.Sleep(350 * time.Millisecond)
		commits := frames.FindAllStringSubmatch(terminal.Text()[offset:], -1)
		if len(commits) == 0 {
			t.Fatal("no synchronized refresh frames observed")
		}
		for _, frame := range commits {
			if strings.Contains(frame[1], "\x1b[2J") && !strings.Contains(frame[1], "POPUP_SENTINEL") {
				t.Fatal("committed a screen clear without the still-open popup")
			}
		}
	}
	offset := len(terminal.Text())
	ctl("refresh-client")
	checkPopup(offset)
	offset = len(terminal.Text())
	terminal.Resize(120, 36)
	checkPopup(offset)
	ctl("display-popup", "-C")
	time.Sleep(150 * time.Millisecond)
	offset = len(terminal.Text())
	ctl("refresh-client")
	terminal.After(offset, "MAIN_SENTINEL")
	time.Sleep(150 * time.Millisecond)
	if strings.Contains(terminal.Text()[offset:], "POPUP_SENTINEL") {
		t.Fatal("closed popup was replayed over a fresh base frame")
	}
	offset = len(terminal.Text())
	ctl("display-menu", "-T", "MENU_SENTINEL", "First", "f", "refresh-client")
	terminal.After(offset, "MENU_SENTINEL")
	terminal.Send("f")
	time.Sleep(150 * time.Millisecond)
	offset = len(terminal.Text())
	ctl("refresh-client")
	terminal.After(offset, "MAIN_SENTINEL")
	time.Sleep(150 * time.Millisecond)
	if strings.Contains(terminal.Text()[offset:], "MENU_SENTINEL") {
		t.Fatal("executed menu was replayed over a fresh base frame")
	}
	ctl("set-window-option", "-t", "popup", "clock-mode-colour", "#123456")
	offset = len(terminal.Text())
	ctl("clock-mode")
	clockMarker := "\x1b[48;2;18;52;86m"
	terminal.After(offset, clockMarker)
	terminal.Send("q")
	deadline := time.Now().Add(2 * time.Second)
	for strings.TrimSpace(ctl("display-message", "-p", "#{pane_in_mode}")) != "0" {
		if time.Now().After(deadline) {
			t.Fatal("clock mode did not exit")
		}
		time.Sleep(10 * time.Millisecond)
	}
	offset = len(terminal.Text())
	ctl("refresh-client")
	terminal.After(offset, "MAIN_SENTINEL")
	time.Sleep(150 * time.Millisecond)
	if strings.Contains(terminal.Text()[offset:], clockMarker) {
		t.Fatal("exited clock mode was replayed over a fresh base frame")
	}
}
