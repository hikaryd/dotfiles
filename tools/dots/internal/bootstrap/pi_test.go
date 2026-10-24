package bootstrap

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fakeCommand(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func fixtureHome(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin)
	return home, bin
}

func TestExistingPiIsReusedWithoutInstalling(t *testing.T) {
	home, bin := fixtureHome(t)
	fakeCommand(t, filepath.Join(bin, "pi"), "printf 'existing-pi\\n'")
	if status := Run("", nil); status != 0 {
		t.Fatalf("exit = %d, want 0", status)
	}
	if _, err := os.Stat(filepath.Join(home, ".local")); !os.IsNotExist(err) {
		t.Fatal("reusing an existing CLI created installation files")
	}
}

func TestManagedPiOutsidePathIsLinked(t *testing.T) {
	home, _ := fixtureHome(t)
	managed := filepath.Join(home, ".pi/agent/bin/pi")
	fakeCommand(t, managed, "printf 'managed-pi\\n'")
	if status := Run("", nil); status != 0 {
		t.Fatalf("exit = %d, want 0", status)
	}
	link := filepath.Join(home, ".local/bin/pi")
	if target, err := os.Readlink(link); err != nil || target != managed {
		t.Fatalf("launcher target = %q, error = %v", target, err)
	}
}

func TestFreshPiInstallIsPinnedUserOwnedAndReused(t *testing.T) {
	home, bin := fixtureHome(t)
	fakeCommand(t, filepath.Join(bin, "node"), "printf 'v22.19.0\\n'")
	fakeCommand(t, filepath.Join(bin, "npm"), `printf '%s\n' "$@" > "$HOME/npm-args"
/bin/mkdir -p "$HOME/.local/bin"
printf '#!/bin/sh\nprintf "new-pi\\n"\n' > "$HOME/.local/bin/pi"
/bin/chmod 755 "$HOME/.local/bin/pi"`)
	if status := Run("", nil); status != 0 {
		t.Fatalf("exit = %d, want 0", status)
	}
	log := filepath.Join(home, "npm-args")
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"install", "-g", "--prefix", filepath.Join(home, ".local"), "--ignore-scripts", "--no-fund", "--no-audit", "@earendil-works/pi-coding-agent@1.1.0"}
	if got := strings.Split(strings.TrimSpace(string(data)), "\n"); !reflect.DeepEqual(got, want) {
		t.Fatalf("npm args = %q, want %q", got, want)
	}
	t.Setenv("PATH", filepath.Join(home, ".local/bin")+string(os.PathListSeparator)+bin)
	if err := os.Remove(log); err != nil {
		t.Fatal(err)
	}
	if status := Run("", nil); status != 0 {
		t.Fatalf("reuse exit = %d", status)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("reusing the installation executed npm again")
	}
}

func TestConflictingLauncherIsPreserved(t *testing.T) {
	home, _ := fixtureHome(t)
	launcher := filepath.Join(home, ".local/bin/pi")
	if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "missing")
	if err := os.Symlink(target, launcher); err != nil {
		t.Fatal(err)
	}
	if status := Run("", nil); status == 0 {
		t.Fatal("conflicting launcher was accepted")
	}
	if got, err := os.Readlink(launcher); err != nil || got != target {
		t.Fatalf("launcher was changed: %q, %v", got, err)
	}
}

func TestMissingOrOldNodeNeverInstalls(t *testing.T) {
	for _, version := range []string{"", "v22.18.0", "v20.19.0", "invalid"} {
		t.Run(version, func(t *testing.T) {
			home, bin := fixtureHome(t)
			fakeCommand(t, filepath.Join(bin, "npm"), `printf 'unexpected\n' > "$HOME/npm-used"`)
			if version != "" {
				fakeCommand(t, filepath.Join(bin, "node"), "printf '"+version+"\\n'")
			}
			if status := Run("", nil); status == 0 {
				t.Fatal("unsupported Node was accepted")
			}
			if _, err := os.Stat(filepath.Join(home, "npm-used")); !os.IsNotExist(err) {
				t.Fatal("npm ran before checking the Node requirement")
			}
		})
	}
}

func TestNPMFailurePropagates(t *testing.T) {
	home, bin := fixtureHome(t)
	fakeCommand(t, filepath.Join(bin, "node"), "printf 'v24.0.0\\n'")
	fakeCommand(t, filepath.Join(bin, "npm"), "exit 42")
	if status := Run("", nil); status != 42 {
		t.Fatalf("exit = %d, want 42", status)
	}
	if _, err := os.Stat(filepath.Join(home, ".local/bin/pi")); !os.IsNotExist(err) {
		t.Fatal("failed npm installation left a launcher")
	}
}

func TestHelpDoesNotExecuteInstalledCLI(t *testing.T) {
	home, bin := fixtureHome(t)
	fakeCommand(t, filepath.Join(bin, "pi"), `printf 'used\n' > "$HOME/pi-used"`)
	if status := Run("", []string{"--help"}); status != 0 {
		t.Fatalf("help exit = %d", status)
	}
	if _, err := os.Stat(filepath.Join(home, "pi-used")); !os.IsNotExist(err) {
		t.Fatal("help executed an installed command")
	}
}
