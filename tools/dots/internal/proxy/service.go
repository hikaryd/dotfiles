package proxy

import (
	"bytes"
	"context"
	"dots/native/internal/launchd"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const serviceLabel = "local.dots.pi-proxy"

func servicePlatform(goos string) error {
	if goos != "darwin" {
		return errors.New("Background proxy service requires macOS launchd.")
	}
	return nil
}
func (c *controller) servicePath() string {
	return filepath.Join(c.home, "Library", "LaunchAgents", serviceLabel+".plist")
}
func serviceDomain() string   { return "gui/" + strconv.Itoa(os.Getuid()) }
func xmlText(s string) string { var b bytes.Buffer; xml.EscapeText(&b, []byte(s)); return b.String() }
func (c *controller) servicePlist(executable string) ([]byte, error) {
	if !filepath.IsAbs(executable) || filepath.Base(executable) != "dots-tools" || strings.Contains(executable, "/go-build") {
		return nil, errors.New("Build/install dots-tools before installing the proxy service; temporary executables are unsupported.")
	}
	path := strings.Join([]string{filepath.Join(c.home, ".local/bin"), filepath.Dir(executable), "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin"}, ":")
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>proxy</string><string>pi</string><string>--daemon</string></array>
<key>EnvironmentVariables</key><dict><key>HOME</key><string>%s</string><key>PATH</key><string>%s</string><key>PI_PROXY_PORT</key><string>%d</string></dict>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
<key>ThrottleInterval</key><integer>30</integer>
<key>AbandonProcessGroup</key><true/>
<key>StandardOutPath</key><string>%s</string>
<key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, serviceLabel, xmlText(executable), xmlText(c.home), xmlText(path), c.port, xmlText(c.path("supervisor.log")), xmlText(c.path("supervisor.log")))), nil
}
func (c *controller) launchctl(args ...string) error {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Discard diagnostics: no launchctl/environment output enters proxy logs.
	return exec.CommandContext(ctx, "launchctl", args...).Run()
}
func (c *controller) serviceLoaded() (bool, error) {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "launchctl", "print", serviceDomain()+"/"+serviceLabel).Output()
	if err == nil {
		argv, ownErr := c.ownedService()
		if ownErr != nil || !launchd.Matches(output, c.servicePath(), argv) {
			return false, errors.New("Loaded proxy job differs; no service changed.")
		}
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 113 {
		return false, nil
	}
	return false, errors.New("Could not inspect the proxy LaunchAgent; existing service was not changed.")
}
func (c *controller) installService() error {
	if err := servicePlatform(runtime.GOOS); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return errors.New("Could not locate installed dots-tools.")
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return errors.New("Could not resolve installed dots-tools.")
	}
	return c.installServiceAt(executable)
}
func (c *controller) installServiceAt(executable string) error {
	if err := servicePlatform(runtime.GOOS); err != nil {
		return err
	}
	if _, err := c.ownedService(); err != nil {
		return err
	}
	data, err := c.servicePlist(executable)
	if err != nil {
		return err
	}
	unlock, err := c.lockFile("service.lock", 5*time.Second)
	if err != nil {
		return err
	}
	defer unlock()
	loaded, err := c.serviceLoaded()
	if err != nil {
		return err
	}
	if loaded {
		current, err := os.ReadFile(c.servicePath())
		if err != nil || !bytes.Equal(current, data) {
			return errors.New("Loaded proxy service definition differs; refusing to restart or replace it.")
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.servicePath()), 0700); err != nil {
		return err
	}
	// launchd preserves permissions of existing log files.
	log, err := os.OpenFile(c.path("supervisor.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	chmodErr := log.Chmod(0600)
	closeErr := log.Close()
	if chmodErr != nil {
		return chmodErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := atomicWrite(c.servicePath(), data, 0600); err != nil {
		return err
	}
	if err := c.launchctl("bootstrap", serviceDomain(), c.servicePath()); err != nil {
		return errors.New("Could not bootstrap the proxy LaunchAgent; the live xray process was not changed.")
	}
	return nil
}
func (c *controller) uninstallService() error {
	// --stop remains usable on other platforms where there is no LaunchAgent.
	if runtime.GOOS != "darwin" {
		return nil
	}
	unlock, err := c.lockFile("service.lock", 5*time.Second)
	if err != nil {
		return err
	}
	defer unlock()
	loaded, err := c.serviceLoaded()
	if err != nil {
		return err
	}
	if _, err := c.ownedService(); err != nil {
		return err
	}
	if loaded {
		if err := c.launchctl("bootout", serviceDomain()+"/"+serviceLabel); err != nil {
			return errors.New("Could not unload proxy supervisor; xray was left running.")
		}
	}
	if err := os.Remove(c.servicePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
