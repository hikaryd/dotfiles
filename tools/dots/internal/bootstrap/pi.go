package bootstrap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func command(name string, args ...string) int {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() > 0 {
			return exit.ExitCode()
		}
		return 1
	}
	return 0
}

func Run(_ string, args []string) int {
	if len(args) != 0 {
		if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
			fmt.Println("Usage: dots-tools setup pi-install")
			return 0
		}
		fmt.Fprintln(os.Stderr, "Usage: dots-tools setup pi-install")
		return 2
	}
	if pi, err := exec.LookPath("pi"); err == nil {
		fmt.Print("Pi is already installed: ")
		return command(pi, "--version")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pi-install: HOME is required")
		return 1
	}
	bin := filepath.Join(home, ".local", "bin")
	launcher := filepath.Join(bin, "pi")
	if _, err := os.Lstat(launcher); !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "pi-install: existing %s is not executable or not on PATH; preserved\n", launcher)
		return 1
	}
	managed := filepath.Join(home, ".pi", "agent", "bin", "pi")
	if info, err := os.Stat(managed); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
		if status := command(managed, "--version"); status != 0 {
			return status
		}
		if err := os.MkdirAll(bin, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "pi-install: cannot create launcher directory")
			return 1
		}
		if err := os.Symlink(managed, launcher); err != nil {
			fmt.Fprintln(os.Stderr, "pi-install: cannot link managed launcher")
			return 1
		}
		return 0
	}
	if _, err := exec.LookPath("node"); err != nil {
		fmt.Fprintln(os.Stderr, "pi-install: Node.js 22.19+ and npm are required (brew install node)")
		return 1
	}
	if _, err := exec.LookPath("npm"); err != nil {
		fmt.Fprintln(os.Stderr, "pi-install: Node.js 22.19+ and npm are required (brew install node)")
		return 1
	}
	version, err := exec.Command("node", "--version").Output()
	if err != nil || !nodeSupported(string(version)) {
		fmt.Fprintln(os.Stderr, "pi-install: Pi requires Node.js 22.19 or newer")
		return 1
	}
	if status := command("npm", "install", "-g", "--prefix", filepath.Join(home, ".local"), "--ignore-scripts", "--no-fund", "--no-audit", "@earendil-works/pi-coding-agent@1.1.0"); status != 0 {
		return status
	}
	return command(launcher, "--version")
}

func nodeSupported(version string) bool {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".")
	if len(parts) < 2 {
		return false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	return majorErr == nil && minorErr == nil && minor >= 0 && (major > 22 || major == 22 && minor >= 19)
}
