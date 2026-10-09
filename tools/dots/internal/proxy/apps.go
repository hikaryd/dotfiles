package proxy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code
		}
	}
	return 1
}
func (c *controller) runCLI(args []string) (int, error) {
	if err := c.requireRunning(); err != nil {
		return 1, err
	}
	bin, err := exec.LookPath("codex")
	if err != nil {
		return 1, errors.New("codex CLI not found. Install it first: npm install -g @openai/codex or brew install --cask codex")
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	// exec.Cmd resolves duplicate environment entries to the last value.
	for _, k := range proxyKeys {
		cmd.Env = append(cmd.Env, k+"="+c.envValue(k))
	}
	cmd.Stdin = c.rawIn
	cmd.Stdout = c.out
	cmd.Stderr = c.errOut
	fmt.Fprintf(c.out, "Starting Codex CLI through proxy: %s\n", c.proxyURL())
	err = cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return 1, errors.New("Could not start Codex CLI.")
		}
	}
	return exitStatus(err), nil
}
func (c *controller) findApp() (string, error) {
	var candidates []string
	if raw := strings.TrimSpace(os.Getenv("CODEX_APP_PATH")); raw != "" {
		if raw == "~" {
			raw = c.home
		} else if strings.HasPrefix(raw, "~/") {
			raw = filepath.Join(c.home, raw[2:])
		}
		candidates = append(candidates, raw)
		if filepath.Base(raw) == "Codex" && strings.Contains(filepath.ToSlash(raw), "Contents/MacOS") {
			candidates = append(candidates, filepath.Dir(filepath.Dir(filepath.Dir(raw))))
		}
	}
	candidates = append(candidates, "/Applications/Codex.app", filepath.Join(c.home, "Applications", "Codex.app"))
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil && filepath.Ext(p) == ".app" {
			return p, nil
		}
	}
	return "", fmt.Errorf("Codex.app bundle not found. Install Codex Desktop first, or set CODEX_APP_PATH to the .app bundle.\nChecked:\n  - %s\n\nExample:\n  CODEX_APP_PATH=\"/Applications/Codex.app\" codex-proxy --run-app", strings.Join(candidates, "\n  - "))
}
func launchctl(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "launchctl", args...).Run()
}
func (c *controller) unsetAppEnv() error {
	if runtime.GOOS != "darwin" {
		return errors.New("--unset-app-env is supported only on macOS.")
	}
	failed := false
	for _, k := range proxyKeys {
		if launchctl("unsetenv", k) != nil {
			failed = true
		}
	}
	if failed {
		return errors.New("Could not remove all proxy variables from launchctl.")
	}
	fmt.Fprintln(c.out, "Removed Codex proxy env from launchctl for future GUI app launches.\nAlready running apps may need to be restarted.")
	return nil
}
func (c *controller) runApp() (int, error) {
	if err := c.requireRunning(); err != nil {
		return 1, err
	}
	app, err := c.findApp()
	if err != nil {
		return 1, err
	}
	if runtime.GOOS != "darwin" {
		return 1, errors.New("--run-app is supported only on macOS.")
	}
	fmt.Fprintln(c.errOut, "Warning: launchctl proxy env affects GUI apps started after this command. Run 'codex-proxy --unset-app-env' after finishing Codex Desktop work.")
	var set []string
	for _, k := range proxyKeys {
		if launchctl("setenv", k, c.envValue(k)) != nil {
			for _, key := range set {
				_ = launchctl("unsetenv", key)
			}
			return 1, errors.New("Could not set launchctl proxy environment; attempted cleanup of variables already set.")
		}
		set = append(set, k)
	}
	fmt.Fprintf(c.out, "Starting Codex.app through proxy: %s\nApp bundle: %s\nIf Codex was already open, quit it first and run this command again.\nAfter finishing, remove GUI proxy env with: codex-proxy --unset-app-env\nThen stop xray with: codex-proxy --stop\n", c.proxyURL(), app)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "open", "-na", app)
	cmd.Stdout = c.out
	cmd.Stderr = c.errOut
	err = cmd.Run()
	if err != nil {
		for _, key := range set {
			_ = launchctl("unsetenv", key)
		}
		return exitStatus(err), errors.New("Could not open Codex.app; attempted cleanup of launchctl proxy environment.")
	}
	return 0, nil
}
