package beatapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"dots/native/internal/tui"
)

const help = `Usage: dots-tools proxy beatapi <command>
  setup [--stdin]    Save BeatAPI key, enable launchd and connect jg
  key [--stdin]      Save/rotate the BeatAPI key
  status             Read-only local configuration/service/health checks
  doctor             One paid Jev request, 15s limit; uses BeatAPI balance
  restart            Restart only the BeatAPI LaunchAgent
  stop               Unload and remove only this LaunchAgent; retain key/jg
  serve [--config ABSOLUTE_PATH]  Run the API in the foreground

API: http://127.0.0.1:6446/v1 (no local authorization).
Only listed model IDs are accepted. jg uses paid jev-1.14 directly.
Legacy jev-1.14-free IDs also route to paid jev-1.14 and spend balance.
Chat models remain free-only. No automatic retries or model fallback.
Outgoing traffic requires the shared dots local HTTP proxy.
Secret input is hidden. Use --stdin explicitly for a password manager.
Do not put a key in command arguments. Setup does not verify upstream inference.`

type cli struct {
	ctx                        context.Context
	home, base, config, binary string
	in                         io.Reader
	out, errOut                io.Writer
	ifConnected                bool
}

// Run is the beatapi subgroup entry point. It never prints credentials.
func Run(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help")) {
		fmt.Fprintln(out, help)
		return 0
	}
	command := args[0]
	stdin, ifConnected := false, false
	config := ""
	switch command {
	case "setup", "key", "status", "doctor", "restart", "stop", "serve":
	default:
		fmt.Fprintln(errOut, "beatapi: unknown command; see --help")
		return 2
	}
	for i := 1; i < len(args); i++ {
		switch {
		case args[i] == "--if-connected" && command == "doctor" && !ifConnected:
			ifConnected = true
		case args[i] == "--stdin" && (command == "setup" || command == "key") && !stdin:
			stdin = true
		case args[i] == "--config" && command == "serve" && config == "" && i+1 < len(args):
			i++
			config = args[i]
			if !filepath.IsAbs(config) {
				fmt.Fprintln(errOut, "beatapi: --config requires an absolute path")
				return 2
			}
		default:
			fmt.Fprintln(errOut, "beatapi: unsupported arguments; see --help")
			return 2
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		fmt.Fprintln(errOut, "beatapi: absolute HOME required")
		return 1
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	if !filepath.IsAbs(base) {
		fmt.Fprintln(errOut, "beatapi: XDG_CONFIG_HOME must be absolute")
		return 1
	}
	if config == "" {
		config = filepath.Join(base, "dots/beatapi-proxy/config.json")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	binary, _ := os.Executable()
	if resolved, e := filepath.EvalSymlinks(binary); e == nil {
		binary = resolved
	}
	c := &cli{ctx: ctx, home: home, base: base, config: config, binary: binary, in: in, out: out, errOut: errOut, ifConnected: ifConnected}
	err = c.execute(command, stdin)
	if errors.Is(err, errNotConnected) {
		return 3 // Wrapper should run the official doctor for a different provider.
	}
	if err != nil {
		if errors.Is(err, tui.ErrCanceled) || ctx.Err() != nil {
			return 130
		}
		fmt.Fprintln(errOut, err)
		return 1
	}
	return 0
}
func (c *cli) execute(command string, stdin bool) error {
	if command == "doctor" {
		cfg, err := loadConfig(c.config)
		if c.ifConnected && !c.jevgrepConnected(cfg) {
			return errNotConnected
		}
		if err != nil {
			return errConfig
		}
		return c.doctor(cfg)
	}
	if command == "status" {
		return c.status()
	}
	if command == "serve" {
		return c.serve()
	}
	if command == "stop" || command == "restart" {
		if runtime.GOOS != "darwin" {
			return errors.New("beatapi: service management requires macOS launchd")
		}
		unlock, err := c.lock()
		if err != nil {
			return err
		}
		defer unlock()
		if command == "stop" {
			return c.stopService()
		}
		return c.restartService()
	}
	if command == "setup" {
		if runtime.GOOS != "darwin" {
			return errors.New("beatapi: setup requires macOS launchd; use serve for foreground use")
		}
		if _, err := exec.LookPath("jg"); err != nil {
			return errors.New("beatapi: install official jevgrep (jg) before setup")
		}
	}
	unlock, err := c.lock()
	if err != nil {
		return err
	}
	defer unlock()
	cfg, err := loadConfig(c.config)
	if err != nil {
		if _, e := os.Lstat(c.config); !errors.Is(e, os.ErrNotExist) {
			return errConfig
		}
		port := 10810
		if value := os.Getenv("PI_PROXY_PORT"); value != "" {
			port, err = strconv.Atoi(value)
			if err != nil || port < 1 || port > 65535 {
				return errors.New("beatapi: PI_PROXY_PORT must be between 1 and 65535")
			}
		}
		cfg = Config{Port: defaultPort, ProxyURL: fmt.Sprintf("http://127.0.0.1:%d", port)}
	}
	if command == "key" || stdin || cfg.APIKey == "" {
		key, err := c.readKey(stdin)
		if err != nil {
			return err
		}
		cfg.APIKey = key
		if key == "" {
			return errors.New("beatapi: a nonempty BeatAPI key is required")
		}
	}
	if err := cfg.validate(); err != nil {
		return err
	}
	if err := saveConfig(c.config, cfg); err != nil {
		return err
	}
	if command == "key" {
		fmt.Fprintln(c.out, "BeatAPI key saved; new requests use it immediately on a compatible running API.")
		return nil
	}
	if err := c.installService(cfg); err != nil {
		return err
	}
	if err := c.waitReady(cfg); err != nil {
		return err
	}
	if err := c.connectJevgrep(cfg); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "BeatAPI service ready: http://%s/v1\njg uses %s through this local API. Upstream access is not yet verified; run dots-tools proxy beatapi doctor.\n", cfg.address(), jevModel)
	return nil
}
func (c *cli) readKey(stdin bool) (string, error) {
	if stdin {
		b, err := io.ReadAll(io.LimitReader(c.in, 8194))
		if err != nil || len(b) > 8193 {
			return "", errors.New("beatapi: cannot read key or input exceeds 8 KiB")
		}
		return strings.TrimSpace(string(b)), nil
	}
	f, ok := c.in.(*os.File)
	if !ok || !tui.IsTerminal(f) {
		return "", errors.New("beatapi: hidden terminal input required; use --stdin explicitly for piped input")
	}
	return tui.SecretInput(c.ctx, tui.Frame{Brand: "dots proxy", Title: "BeatAPI API key", Mode: "LOCAL CONFIG", Tone: tui.Yellow, Intro: "Ввод скрыт. Ключ хранится только в приватном конфиге. Esc — отмена."}, "")
}
func (c *cli) serve() error {
	cfg, err := loadConfig(c.config)
	if err != nil {
		return errConfig
	}
	client, err := upstreamClient(cfg.ProxyURL)
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()
	load := func() (Config, error) {
		latest, err := loadConfig(c.config)
		if err != nil || latest.Port != cfg.Port || latest.ProxyURL != cfg.ProxyURL {
			return Config{}, errConfig
		}
		return latest, nil
	}
	listener, err := net.Listen("tcp4", cfg.address())
	if err != nil {
		return errors.New("beatapi: local API port unavailable; no existing process was changed")
	}
	server := &http.Server{Handler: newHandler(load, client, upstreamURL), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10, ErrorLog: log.New(io.Discard, "", 0)}
	ended := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-c.ctx.Done():
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if server.Shutdown(ctx) != nil {
				server.Close()
			}
		case <-ended:
		}
	}()
	err = server.Serve(listener)
	close(ended)
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return errors.New("beatapi: local HTTP server stopped unexpectedly")
}
func localHealth(cfg Config) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+cfg.address()+"/health", nil)
	// Never inherit HTTP_PROXY, including while checking a local service.
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return false
	}
	var health struct {
		Service      string
		Configured   bool
		Capabilities []string
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&health) == nil && health.Service == label && health.Configured && slices.Contains(health.Capabilities, "beatapi")
}
func (c *cli) waitReady(cfg Config) error {
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if localHealth(cfg) {
			return nil
		}
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-deadline.C:
			return errors.New("beatapi: local API is not ready or lacks BeatAPI support; rebuild, run dots-tools proxy beatapi restart and repeat setup (jg unchanged)")
		case <-tick.C:
		}
	}
}
func (c *cli) status() error {
	cfg, configErr := loadConfig(c.config)
	loaded, serviceErr := c.serviceLoaded()
	healthy := false
	if configErr == nil {
		healthy = localHealth(cfg)
	}
	state := "not loaded"
	if loaded {
		state = "loaded"
	}
	if serviceErr != nil {
		state = "unavailable"
	}
	fmt.Fprintf(c.out, "BeatAPI configuration: %t\nLaunchAgent %s: %s\nLocal API healthy: %t\n", configErr == nil, label, state, healthy)
	if configErr != nil || serviceErr != nil || !loaded || !healthy {
		return errors.New("beatapi: not ready; run setup (no changes made by status)")
	}
	return nil
}
