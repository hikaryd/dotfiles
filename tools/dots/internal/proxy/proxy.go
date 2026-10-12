// Package proxy provides the Codex xray controller and Spotify preference helper.
package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"dots/native/internal/tui"
)

// Nebula's internal MCP endpoint must use the corporate network, not VLESS.
const noProxy = "127.0.0.1,localhost,::1,*.local,nebula-staging.x5.ru"

var proxyKeys = []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy", "WS_PROXY", "WSS_PROXY", "ws_proxy", "wss_proxy", "NO_PROXY", "no_proxy"}

type controller struct {
	home, dir   string
	port        int
	in          *bufio.Reader
	rawIn       io.Reader
	ctx         context.Context
	out, errOut io.Writer
}

// Run dispatches codex followed by its original flags, or spotify --port/--check.
func Run(root string, args []string) int { return run(root, args, os.Stdin, os.Stdout, os.Stderr) }

func run(_ string, args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: dots-tools proxy {codex|spotify}")
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		fmt.Fprintln(errOut, "proxy: HOME is not configured")
		return 2
	}
	if args[0] == "spotify" {
		return runSpotify(home, args[1:], out, errOut)
	}
	if args[0] != "codex" {
		fmt.Fprintln(errOut, "usage: dots-tools proxy {codex|spotify}")
		return 2
	}
	port := 10810
	if s, ok := os.LookupEnv("CODEX_PROXY_PORT"); ok {
		port, err = strconv.Atoi(s)
	}
	if err != nil || port < 1 || port > 65535 {
		fmt.Fprintln(errOut, "CODEX_PROXY_PORT must be between 1 and 65535")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	c := &controller{ctx: ctx, home: home, dir: filepath.Join(home, ".config", "xray-codex"), port: port, in: bufio.NewReader(in), rawIn: in, out: out, errOut: errOut}
	if err = c.harden(); err != nil {
		fmt.Fprintln(errOut, "codex-proxy: could not secure configuration")
		return 1
	}
	code, err := c.dispatch(args[1:])
	if err != nil {
		if errors.Is(err, tui.ErrCanceled) || errors.Is(err, context.Canceled) {
			return 130
		}
		fmt.Fprintln(errOut, err)
		if code == 0 {
			code = 1
		}
	}
	return code
}

func (c *controller) path(name string) string { return filepath.Join(c.dir, name) }
func (c *controller) envPath() string         { return filepath.Join(c.home, ".codex", ".env") }
func (c *controller) proxyURL() string        { return fmt.Sprintf("http://127.0.0.1:%d", c.port) }
func (c *controller) envValue(key string) string {
	if key == "NO_PROXY" || key == "no_proxy" {
		return noProxy
	}
	return c.proxyURL()
}
func (c *controller) printEnv() {
	for _, key := range proxyKeys {
		fmt.Fprintf(c.out, "export %s=%q\n", key, c.envValue(key))
	}
}

func (c *controller) harden() error {
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(c.dir, 0700); err != nil {
		return err
	}
	for _, path := range []string{c.path("subscription_url.txt"), c.path("servers.json"), c.path("config.json"), c.path("xray.pid"), c.envPath()} {
		if err := os.Chmod(path, 0600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
func readText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(b) {
		return "", errors.New("invalid UTF-8")
	}
	return strings.ReplaceAll(strings.ReplaceAll(string(b), "\r\n", "\n"), "\r", "\n"), nil
}
func optionalText(path string) (string, error) {
	s, err := readText(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return s, err
}
func (c *controller) interactive() bool {
	file, ok := c.rawIn.(*os.File)
	return ok && tui.IsTerminal(file)
}
func (c *controller) prompt(prompt string) (string, error) {
	if c.interactive() {
		return tui.SecretInput(c.ctx, tui.Frame{Brand: "dots proxy", Title: prompt, Mode: "LOCAL CONFIG", Tone: tui.Yellow, Intro: "Ввод скрыт. Esc — отмена; Ctrl+L — очистить."}, "")
	}
	// Keep the explicit stdin protocol for scripted provisioning.
	fmt.Fprint(c.out, prompt)
	s, err := c.in.ReadString('\n')
	if errors.Is(err, io.EOF) && s != "" {
		err = nil
	}
	return strings.TrimSpace(s), err
}
func (c *controller) installEnv() error {
	const start = "# >>> codex-proxy managed block >>>"
	const end = "# <<< codex-proxy managed block <<<"
	existing, err := optionalText(c.envPath())
	if err != nil {
		return errors.New("codex-proxy: could not read Codex env")
	}
	var block strings.Builder
	block.WriteString(start + "\n")
	for _, k := range proxyKeys {
		fmt.Fprintf(&block, "%s=%q\n", k, c.envValue(k))
	}
	block.WriteString(end + "\n")
	content := block.String()
	if before, rest, ok := strings.Cut(existing, start); ok && strings.Contains(rest, end) {
		_, after, _ := strings.Cut(rest, end)
		content = strings.TrimRight(before, " \t\r\n") + "\n\n" + content + strings.TrimLeft(after, " \t\r\n")
	} else if strings.TrimSpace(existing) != "" {
		content = strings.TrimRight(existing, " \t\r\n") + "\n\n" + content
	}
	if err := atomicWrite(c.envPath(), []byte(content), 0600); err != nil {
		return errors.New("codex-proxy: could not write Codex env")
	}
	fmt.Fprintf(c.out, "Codex env written: %s\nRestart Codex CLI/IDE extension after this change.\nIf Codex.app was opened from Finder and ignores .env, use: codex-proxy --run-app\n", c.envPath())
	return nil
}

func (c *controller) dispatch(args []string) (int, error) {
	has := func(flag string) bool {
		for _, a := range args {
			if a == flag {
				return true
			}
		}
		return false
	}
	switch {
	case has("--help") || has("-h"):
		fmt.Fprintln(c.out, "\n"+helpText)
	case has("--set-vless"):
		return 0, c.setSingle()
	case has("--set-sub"):
		s, err := c.prompt("Enter subscription URL: ")
		if err != nil {
			return 1, err
		}
		if err := c.saveSubscription(s); err != nil {
			return 1, err
		}
		_, err = c.updateServers(has("--allow-insecure-sub") || truthy(os.Getenv("CODEX_PROXY_ALLOW_INSECURE_SUB")))
		return 0, err
	case has("--update"):
		_, err := c.updateServers(has("--allow-insecure-sub") || truthy(os.Getenv("CODEX_PROXY_ALLOW_INSECURE_SUB")))
		return 0, err
	case has("--stop"):
		return 0, c.stop()
	case has("--status"):
		return 0, c.status()
	case has("--install-codex-env"):
		return 0, c.installEnv()
	case has("--print-env"):
		c.printEnv()
	case has("--list"):
		servers, err := c.loadServers()
		if err != nil {
			return 1, err
		}
		c.display(servers)
	case has("--unset-app-env"):
		return 0, c.unsetAppEnv()
	case has("--run-app"):
		return c.runApp()
	case has("--run"):
		for i, arg := range args {
			if arg == "--run" {
				rest := args[i+1:]
				if len(rest) > 0 && rest[0] == "--" {
					rest = rest[1:]
				}
				return c.runCLI(rest)
			}
		}
	default:
		return 0, c.selectAndStart()
	}
	return 0, nil
}
