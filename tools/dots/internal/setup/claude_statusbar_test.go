package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var statusbarFiles = []string{"commands/statusbar.md", "commands/statusbar-doctor.md", "commands/statusbar-preview.md", "commands/statusbar-style.md", "commands/statusbar-reset.md", "skills/claude-statusbar/SKILL.md"}

func statusbarFixture(t *testing.T) (string, string, string) {
	t.Helper()
	h := filepath.Join(t.TempDir(), "home with 'quote")
	t.Setenv("HOME", h)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	for _, f := range statusbarFiles {
		put(t, filepath.Join(h, ".claude", f), "source "+f)
	}
	guard := filepath.Join(h, ".local/share/claude-guard/home/.claude")
	custom := filepath.Join(h, "custom profile")
	return h, guard, custom
}

func TestClaudeStatusbarProfiles(t *testing.T) {
	h, guard, custom := statusbarFixture(t)
	t.Setenv("CLAUDE_CONFIG_DIR", custom)
	original := `{"hooks":{"PreToolUse":[{"x":1}]},"model":"keep","plugins":{"x":true}}`
	for _, p := range []string{guard, custom} {
		put(t, filepath.Join(p, "settings.json"), original)
		put(t, filepath.Join(p, "commands/statusbar.md"), "old")
		put(t, filepath.Join(p, "commands/unrelated.md"), "keep")
	}
	if got := Run("", []string{"claude-statusbar-apply"}); got != 0 {
		t.Fatal(got)
	}
	for _, p := range []string{guard, custom} {
		obj, err := readObject(filepath.Join(p, "settings.json"), false)
		if err != nil {
			t.Fatal(err)
		}
		if obj["model"] != "keep" || obj["hooks"] == nil || obj["plugins"] == nil {
			t.Fatal(obj)
		}
		status := obj["statusLine"].(map[string]any)
		cmd := status["command"].(string)
		expected := filepath.Join(h, ".local/bin/cs")
		out, err := exec.Command("bash", "-c", "set -- "+cmd+"; printf '%s\\n%s' \"$1\" \"$2\"").Output()
		if err != nil || string(out) != expected+"\nrender" {
			t.Fatalf("bad command %q: %q %v", cmd, out, err)
		}
		if status["type"] != "command" || string(status["refreshInterval"].(interface{ String() string }).String()) != "1" {
			t.Fatal(status)
		}
		for _, f := range statusbarFiles {
			if get(t, filepath.Join(p, f)) != "source "+f {
				t.Fatal(f)
			}
		}
		if get(t, filepath.Join(p, "commands/unrelated.md")) != "keep" {
			t.Fatal("unrelated changed")
		}
	}
	before := tree(t, h)
	if err := claudeStatusbarApply(); err != nil {
		t.Fatal(err)
	}
	if after := tree(t, h); !reflect.DeepEqual(before, after) {
		t.Fatal("repeat changed files or backups")
	}
	backup := filepath.Join(h, ".config/dots-backups/claude-statusbar")
	count := 0
	err := filepath.Walk(backup, func(_ string, i os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if !i.IsDir() {
			count++
			if i.Mode().Perm() != 0600 {
				t.Errorf("backup mode %v", i.Mode())
			}
		}
		return nil
	})
	if err != nil || count != 4 {
		t.Fatalf("backups %d %v", count, err)
	}
}

func TestClaudeStatusbarNoGuardAndDedup(t *testing.T) {
	h, guard, _ := statusbarFixture(t)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(h, ".claude"))
	before := tree(t, h)
	if err := claudeStatusbarApply(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(guard); !os.IsNotExist(err) {
		t.Fatal("guard created", err)
	}
	if !reflect.DeepEqual(before, tree(t, h)) {
		t.Fatal("standard profile changed")
	}
}

func TestClaudeStatusbarPreflight(t *testing.T) {
	h, _, _ := statusbarFixture(t)
	p := filepath.Join(h, ".claude/settings.json")
	put(t, p, "{")
	if claudeStatusbarPreflight() == nil {
		t.Fatal("malformed settings accepted")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(h, ".claude/commands/statusbar.md"), p); err != nil {
		t.Fatal(err)
	}
	if claudeStatusbarPreflight() == nil {
		t.Fatal("symlink settings accepted")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	put(t, p, `{"statusLine":{"command":"foreign"}}`)
	before := tree(t, h)
	if err := claudeStatusbarPreflight(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, tree(t, h)) {
		t.Fatal("preflight changed files")
	}
}

func TestClaudeStatusbarPreflightProfiles(t *testing.T) {
	for _, scenario := range []string{"guard malformed", "custom symlink", "standard directory"} {
		t.Run(scenario, func(t *testing.T) {
			h, guard, custom := statusbarFixture(t)
			t.Setenv("CLAUDE_CONFIG_DIR", custom)
			switch scenario {
			case "guard malformed":
				put(t, filepath.Join(guard, "settings.json"), "{")
			case "custom symlink":
				p := filepath.Join(custom, statusbarFiles[0])
				if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(h, ".claude", statusbarFiles[0]), p); err != nil {
					t.Fatal(err)
				}
			case "standard directory":
				p := filepath.Join(h, ".claude", statusbarFiles[0])
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			before := tree(t, h)
			if err := claudeStatusbarPreflight(); err == nil {
				t.Fatal("invalid profile accepted")
			}
			if !reflect.DeepEqual(before, tree(t, h)) {
				t.Fatal("preflight changed files")
			}
		})
	}
}

func TestClaudeStatusbarRejectsWithoutPublication(t *testing.T) {
	for _, scenario := range []string{"malformed", "missing source", "settings symlink", "file symlink", "directory conflict"} {
		t.Run(scenario, func(t *testing.T) {
			h, guard, custom := statusbarFixture(t)
			t.Setenv("CLAUDE_CONFIG_DIR", custom)
			put(t, filepath.Join(custom, "settings.json"), `{"model":"preserve"}`)
			put(t, filepath.Join(guard, "settings.json"), `{"hooks":{}}`)
			switch scenario {
			case "malformed":
				put(t, filepath.Join(guard, "settings.json"), "{")
			case "missing source":
				if err := os.Remove(filepath.Join(h, ".claude", statusbarFiles[5])); err != nil {
					t.Fatal(err)
				}
			case "settings symlink":
				os.Remove(filepath.Join(guard, "settings.json"))
				if err := os.Symlink(filepath.Join(custom, "settings.json"), filepath.Join(guard, "settings.json")); err != nil {
					t.Fatal(err)
				}
			case "file symlink":
				os.MkdirAll(filepath.Join(guard, "commands"), 0700)
				if err := os.Symlink(filepath.Join(custom, "settings.json"), filepath.Join(guard, statusbarFiles[0])); err != nil {
					t.Fatal(err)
				}
			case "directory conflict":
				if err := os.MkdirAll(filepath.Join(guard, statusbarFiles[0]), 0700); err != nil {
					t.Fatal(err)
				}
			}
			before := tree(t, h)
			if err := claudeStatusbarApply(); err == nil {
				t.Fatal("expected error")
			}
			if !reflect.DeepEqual(before, tree(t, h)) {
				t.Fatal("partial changes")
			}
		})
	}
}

func TestClaudeStatusbarInstallerOffline(t *testing.T) {
	root := t.TempDir()
	h := filepath.Join(root, "home")
	bin := filepath.Join(root, "fakebin")
	if err := os.MkdirAll(h, 0700); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "scripts", "claude-statusbar-install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(root, "scripts/install.sh"), string(script))
	put(t, filepath.Join(root, "build/bin/dots-tools"), "#!/bin/bash\nprintf '%s\\n' \"$*\" >> \"$HOME/log\"\n")
	put(t, filepath.Join(bin, "curl"), `#!/bin/bash
mkdir -p "$HOME"
echo curl >> "$HOME/log"
cat > "$4" <<'INSTALL'
#!/bin/bash
mkdir -p "$HOME/.local/bin"
cat > "$HOME/.local/bin/cs" <<'CS'
#!/bin/bash
echo "cs $*" >> "$HOME/log"
CS
chmod +x "$HOME/.local/bin/cs"
INSTALL
`)
	for _, p := range []string{filepath.Join(root, "build/bin/dots-tools"), filepath.Join(bin, "curl")} {
		if err := os.Chmod(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		c := exec.Command("bash", filepath.Join(root, "scripts/install.sh"))
		c.Env = append(os.Environ(), "HOME="+h, "PATH="+bin+":"+os.Getenv("PATH"))
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	log := get(t, filepath.Join(h, "log"))
	if strings.Count(log, "curl") != 1 || strings.Count(log, "cs --setup") != 2 || strings.Count(log, "setup claude-statusbar-apply") != 2 {
		t.Fatal(log)
	}
	if err := os.Remove(filepath.Join(h, ".local/bin/cs")); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(bin, "curl"), "#!/bin/bash\nprintf 'touch \"$HOME/partial-ran\"\\n' > \"$4\"\nexit 22\n")
	if err := os.Chmod(filepath.Join(bin, "curl"), 0700); err != nil {
		t.Fatal(err)
	}
	c := exec.Command("bash", filepath.Join(root, "scripts/install.sh"))
	c.Env = append(os.Environ(), "HOME="+h, "PATH="+bin+":"+os.Getenv("PATH"))
	if err := c.Run(); err == nil {
		t.Fatal("failed download accepted")
	}
	if _, err := os.Stat(filepath.Join(h, "partial-ran")); !os.IsNotExist(err) {
		t.Fatal("partial download executed")
	}
	put(t, filepath.Join(root, "build/bin/dots-tools"), "#!/bin/bash\nexit 1\n")
	put(t, filepath.Join(bin, "curl"), "#!/bin/bash\necho curl >> \"$HOME/log\"\nexit 1\n")
	if err := os.Chmod(filepath.Join(root, "build/bin/dots-tools"), 0700); err != nil {
		t.Fatal(err)
	}
	prior := get(t, filepath.Join(h, "log"))
	c = exec.Command("bash", filepath.Join(root, "scripts/install.sh"))
	c.Env = append(os.Environ(), "HOME="+h, "PATH="+bin+":"+os.Getenv("PATH"))
	if err := c.Run(); err == nil {
		t.Fatal("failed preflight accepted")
	}
	if get(t, filepath.Join(h, "log")) != prior {
		t.Fatal("curl or cs ran after failed preflight")
	}
	put(t, filepath.Join(h, ".local/bin/cs"), "#!/bin/bash\necho cs >> \"$HOME/log\"\n")
	if err := os.Chmod(filepath.Join(h, ".local/bin/cs"), 0700); err != nil {
		t.Fatal(err)
	}
	c = exec.Command("bash", filepath.Join(root, "scripts/install.sh"))
	c.Env = append(os.Environ(), "HOME="+h, "PATH="+bin+":"+os.Getenv("PATH"))
	if err := c.Run(); err == nil {
		t.Fatal("failed preflight accepted")
	}
	if get(t, filepath.Join(h, "log")) != prior {
		t.Fatal("cs ran after failed preflight")
	}
}
