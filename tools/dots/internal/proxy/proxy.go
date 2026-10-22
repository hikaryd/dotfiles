// Package proxy provides the Pi xray controller.
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

	"dots/native/internal/opencode"
	"dots/native/internal/tui"
)

// Nebula's internal MCP endpoint must use the corporate network, not VLESS.
const noProxy = "127.0.0.1,localhost,::1,*.local,nebula-staging.x5.ru"

var proxyKeys = []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy", "WS_PROXY", "WSS_PROXY", "ws_proxy", "wss_proxy", "NO_PROXY", "no_proxy"}

type controller struct {
	home, dir                string
	countryTarget, apiTarget string
	port                     int
	in                       *bufio.Reader
	rawIn                    io.Reader
	ctx                      context.Context
	out, errOut              io.Writer
}

// Run dispatches independent opencode and Pi xray controllers.
func Run(root string, args []string) int { return run(root, args, os.Stdin, os.Stdout, os.Stderr) }

func run(_ string, args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) > 0 && args[0] == "opencode" {
		return opencode.Run(args[1:], in, out, errOut)
	}
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: dots-tools proxy pi|opencode")
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		fmt.Fprintln(errOut, "proxy: HOME is not configured")
		return 2
	}
	if args[0] != "pi" {
		fmt.Fprintln(errOut, "usage: dots-tools proxy pi|opencode")
		return 2
	}
	port := 10810
	if s, ok := os.LookupEnv("PI_PROXY_PORT"); ok {
		port, err = strconv.Atoi(s)
	}
	if err != nil || port < 1 || port > 65535 {
		fmt.Fprintln(errOut, "PI_PROXY_PORT must be between 1 and 65535")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	c := &controller{ctx: ctx, home: home, dir: filepath.Join(home, ".config", "xray-pi"), port: port, in: bufio.NewReader(in), rawIn: in, out: out, errOut: errOut}
	// The consumer readiness path must not create/chmod state or acquire locks.
	if len(args) == 2 && (args[1] == "--require-ready" || args[1] == "--ensure") {
		if err := c.requireReady(); err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		return 0
	}
	if err = c.harden(); err != nil {
		fmt.Fprintln(errOut, "pi-proxy: could not secure configuration")
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
	for _, path := range []string{c.path("subscription_url.txt"), c.path("servers.json"), c.path("config.json"), c.path("xray.pid"), c.path("selection.json")} {
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
func (c *controller) dispatch(args []string) (int, error) {
	for _, arg := range args {
		if arg == "--run" {
			break // All following words belong to Pi.
		}
		switch arg {
		case "--help", "-h", "--set-vless", "--set-sub", "--update", "--allow-insecure-sub", "--stop", "--status", "--print-env", "--list", "--auto", "--ensure", "--daemon", "--require-ready", "--install-service", "--uninstall-service":
		default:
			return 2, errors.New("unsupported proxy option; see pi-proxy --help")
		}
	}
	has := func(flag string) bool {
		for _, a := range args {
			if a == flag {
				return true
			}
			if a == "--run" {
				break // Pi flags must not be interpreted as controller options.
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
		_, err = c.updateServers(has("--allow-insecure-sub") || truthy(os.Getenv("PI_PROXY_ALLOW_INSECURE_SUB")))
		return 0, err
	case has("--update"):
		_, err := c.updateServers(has("--allow-insecure-sub") || truthy(os.Getenv("PI_PROXY_ALLOW_INSECURE_SUB")))
		return 0, err
	case has("--daemon"):
		return 0, c.daemon()
	case has("--require-ready"):
		return 0, c.requireReady()
	case has("--install-service"):
		return 0, c.installService()
	case has("--uninstall-service"):
		return 0, c.uninstallService()
	case has("--auto"):
		return 0, c.autoStart(true)
	case has("--ensure"):
		return 0, c.requireReady()
	case has("--stop"):
		if err := c.uninstallService(); err != nil {
			return 1, err
		}
		return 0, c.stop()
	case has("--status"):
		return 0, c.status()
	case has("--print-env"):
		c.printEnv()
	case has("--list"):
		servers, err := c.loadServers()
		if err != nil {
			return 1, err
		}
		c.display(servers)
	case has("--run"):
		for i, arg := range args {
			if arg == "--run" {
				rest := args[i+1:]
				if len(rest) > 0 && rest[0] == "--" {
					rest = rest[1:]
				}
				if err := c.requireReady(); err != nil {
					return 1, err
				}
				return c.runCLI(rest)
			}
		}
	default:
		return 0, c.selectAndStart()
	}
	return 0, nil
}
