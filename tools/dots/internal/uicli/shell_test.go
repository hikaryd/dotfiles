package uicli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dots/native/internal/testpty"
)

func shellFixture(t *testing.T) (string, []string) {
	t.Helper()
	root := t.TempDir()
	source, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	files := []string{"config/tmux/scripts/sesh-picker.sh", "config/ghostty/scripts/gtab-pick.sh", "config/zsh/functions/_dots-zi", "config/zsh/functions/_dots-zoxide-init", "scripts/terminal-ui.sh", "vpn/install.sh", "uninstall"}
	for _, file := range files {
		b, err := os.ReadFile(filepath.Join(source, file))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, file)
		os.MkdirAll(filepath.Dir(path), 0700)
		if err := os.WriteFile(path, b, 0700); err != nil {
			t.Fatal(err)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "build/bin")
	os.MkdirAll(bin, 0700)
	if err := os.Symlink(exe, filepath.Join(bin, "dots-tools")); err != nil {
		t.Fatal(err)
	}
	fake := `#!/bin/bash
case "$(basename "$0"):$1" in
 sesh:list) printf '%s\n' "first session" "second session" ;;
 zoxide:init) printf '%s\n' 'function z() { :; }' 'function zi() { print external-ui; }' ;;
 zoxide:query) printf '%s\n' "$SHELL_DIR" ;;
 *) printf '%s\n' "$@" > "$SHELL_CALLS" ;;
esac
`
	for _, name := range []string{"sesh", "gtab", "zoxide", "ssh", "curl"} {
		os.WriteFile(filepath.Join(bin, name), []byte(fake), 0700)
	}
	home := filepath.Join(root, "home")
	os.MkdirAll(home, 0700)
	env := []string{"HOME=" + home, "DOTS_ROOT=" + root, "PATH=" + bin + ":/bin:/usr/bin", "DOTS_UI_CLI=1", "NO_COLOR=1", "TERM=xterm-256color", "GORACE=atexit_sleep_ms=0", "SHELL_CALLS=" + filepath.Join(root, "calls"), "SHELL_DIR=" + home}
	return root, env
}
func TestShellPickersUseNativeUIAndOriginalArgv(t *testing.T) {
	for _, kind := range []string{"sesh", "gtab", "zi", "zi-echo"} {
		t.Run(kind, func(t *testing.T) {
			root, env := shellFixture(t)
			var cmd *exec.Cmd
			title := ""
			selection := ""
			switch kind {
			case "sesh":
				cmd = exec.Command("/bin/bash", filepath.Join(root, "config/tmux/scripts/sesh-picker.sh"))
				title = "Sesh"
				selection = "second"
			case "gtab":
				dir := filepath.Join(root, "gtab")
				os.MkdirAll(dir, 0700)
				os.WriteFile(filepath.Join(dir, "space name.applescript"), nil, 0600)
				env = append(env, "GTAB_DIR="+dir)
				cmd = exec.Command("/bin/bash", filepath.Join(root, "config/ghostty/scripts/gtab-pick.sh"))
				title = "Gtab"
				selection = "space"
			case "zi", "zi-echo":
				if kind == "zi-echo" {
					env = append(env, "_ZO_ECHO=1")
				}
				zsh, err := exec.LookPath("zsh")
				if err != nil {
					t.Skip("zsh not installed")
				}
				cmd = exec.Command(zsh, "-f", "-c", `fpath=("$DOTS_ROOT/config/zsh/functions" $fpath); autoload -Uz _dots-zoxide-init; _dots-zoxide-init; zi || exit $?; print -r -- "$PWD" > "$SHELL_CALLS"`)
				title = "zoxide"
			}
			cmd.Env = env
			p := testpty.Start(t, cmd)
			p.Screen(title)
			if selection != "" {
				p.Send(selection)
				p.Screen("Поиск › " + selection)
			}
			p.Send("\r")
			p.Wait(0)
			b, err := os.ReadFile(filepath.Join(root, "calls"))
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"sesh": "connect\nsecond session\n", "gtab": "space name\n", "zi": filepath.Join(root, "home") + "\n", "zi-echo": filepath.Join(root, "home") + "\n"}[kind]
			if string(b) != want {
				t.Fatalf("argv/cwd %q want %q", b, want)
			}
		})
	}
}
func TestShellInstallersCancelBeforeExternalActions(t *testing.T) {
	for _, installer := range []string{"vpn/install.sh", "uninstall"} {
		root, env := shellFixture(t)
		cmd := exec.Command("/bin/bash", filepath.Join(root, installer))
		cmd.Env = env
		p := testpty.Start(t, cmd)
		code := 130
		if installer == "uninstall" {
			p.Screen("Введите yes")
			code = 1
		} else {
			p.Screen("Ссылка на подписку")
		}
		p.Send("\x1b")
		p.Wait(code)
		if _, err := os.Stat(filepath.Join(root, "calls")); !os.IsNotExist(err) {
			t.Fatal("cancel called external tool")
		}
		entries, err := os.ReadDir(filepath.Join(root, "home"))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.Contains(entry.Name(), "backup") {
				t.Fatal("cancel changed files")
			}
		}
	}
}
