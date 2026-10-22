package opencode

import (
	"bytes"
	"context"
	"dots/native/internal/launchd"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (c *cli) servicePath() string {
	return filepath.Join(c.home, "Library/LaunchAgents", label+".plist")
}
func serviceDomain() string   { return "gui/" + strconv.Itoa(os.Getuid()) }
func xmlText(s string) string { var b bytes.Buffer; xml.EscapeText(&b, []byte(s)); return b.String() }
func (c *cli) servicePlist() ([]byte, error) {
	if !filepath.IsAbs(c.binary) || filepath.Base(c.binary) != "dots-tools" || strings.Contains(c.binary, "go-build") || !filepath.IsAbs(c.config) {
		return nil, errors.New("opencode: build/install dots-tools using scripts/build-go-tools.sh before setup")
	}
	st, err := os.Stat(c.binary)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0111 == 0 {
		return nil, errors.New("opencode: installed dots-tools executable is unavailable")
	}
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>proxy</string><string>opencode</string><string>serve</string><string>--config</string><string>%s</string></array>
<key>EnvironmentVariables</key><dict><key>HOME</key><string>%s</string><key>XDG_CONFIG_HOME</key><string>%s</string></dict>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
<key>ThrottleInterval</key><integer>30</integer>
<key>StandardOutPath</key><string>/dev/null</string>
<key>StandardErrorPath</key><string>/dev/null</string>
</dict></plist>
`, label, xmlText(c.binary), xmlText(c.config), xmlText(c.home), xmlText(c.base))), nil
}
func (c *cli) launchctl(args ...string) ([]byte, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("opencode: launchd requires macOS")
	}
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "launchctl", args...).Output()
}
func (c *cli) serviceLoaded() (bool, error) {
	output, err := c.launchctl("print", serviceDomain()+"/"+label)
	if err == nil {
		if !c.matchesLoadedJob(output) {
			return false, errors.New("opencode: loaded job definition differs; no service changed")
		}
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 113 {
		return false, nil
	}
	return false, errors.New("opencode: cannot inspect own LaunchAgent; no service changed")
}

// A matching file on disk alone cannot prove ownership of a loaded label.
func (c *cli) matchesLoadedJob(output []byte) bool {
	return launchd.Matches(output, c.servicePath(), []string{c.binary, "proxy", "opencode", "serve", "--config", c.config})
}
func (c *cli) ownedPlist(expected []byte) (bool, error) {
	if err := checkPath(c.servicePath()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, errors.New("opencode: unsafe LaunchAgent file; refusing to replace it")
	}
	st, err := os.Lstat(c.servicePath())
	if err != nil || !st.Mode().IsRegular() {
		return false, errors.New("opencode: unsafe LaunchAgent file")
	}
	b, err := os.ReadFile(c.servicePath())
	if err != nil || !bytes.Equal(b, expected) {
		return false, errors.New("opencode: existing LaunchAgent definition differs; refusing to replace or stop it")
	}
	return true, nil
}
func (c *cli) installService(cfg Config) error {
	data, err := c.servicePlist()
	if err != nil {
		return err
	}
	exists, err := c.ownedPlist(data)
	if err != nil {
		return err
	}
	loaded, err := c.serviceLoaded()
	if err != nil {
		return err
	}
	if loaded {
		if !exists {
			return errors.New("opencode: loaded service has no matching plist; no service changed")
		}
		return nil
	}
	listener, err := net.Listen("tcp4", cfg.address())
	if err != nil {
		return errors.New("opencode: API port is occupied; no existing process was stopped")
	}
	listener.Close()
	dir := filepath.Dir(c.servicePath())
	if err := checkPath(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("opencode: unsafe LaunchAgents directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.New("opencode: cannot create LaunchAgents directory")
	}
	if !exists {
		if err := atomicPrivate(c.servicePath(), data); err != nil {
			return errors.New("opencode: cannot write own LaunchAgent")
		}
	}
	if _, err := c.launchctl("bootstrap", serviceDomain(), c.servicePath()); err != nil {
		return errors.New("opencode: cannot bootstrap LaunchAgent; key retained, jg unchanged")
	}
	return nil
}
func (c *cli) stopService() error {
	data, err := c.servicePlist()
	if err != nil {
		return err
	}
	exists, err := c.ownedPlist(data)
	if err != nil {
		return err
	}
	loaded, err := c.serviceLoaded()
	if err != nil {
		return err
	}
	if loaded {
		if !exists {
			return errors.New("opencode: loaded service has no matching plist; refusing to stop it")
		}
		if _, err := c.launchctl("bootout", serviceDomain()+"/"+label); err != nil {
			return errors.New("opencode: cannot unload own LaunchAgent")
		}
	}
	if exists {
		if os.Remove(c.servicePath()) != nil {
			return errors.New("opencode: cannot remove own LaunchAgent")
		}
	}
	fmt.Fprintln(c.out, "OpenCode LaunchAgent stopped; configuration and jg credentials retained.")
	return nil
}
func (c *cli) restartService() error {
	data, err := c.servicePlist()
	if err != nil {
		return err
	}
	exists, err := c.ownedPlist(data)
	if err != nil || !exists {
		return errors.New("opencode: matching LaunchAgent required; run setup")
	}
	loaded, err := c.serviceLoaded()
	if err != nil {
		return err
	}
	if !loaded {
		return errors.New("opencode: LaunchAgent is not loaded; run setup")
	}
	if _, err := c.launchctl("kickstart", "-k", serviceDomain()+"/"+label); err != nil {
		return errors.New("opencode: cannot restart own LaunchAgent")
	}
	fmt.Fprintln(c.out, "OpenCode LaunchAgent restarted.")
	return nil
}
func (c *cli) lock() (func(), error) {
	dir := filepath.Dir(c.config)
	if privateDir(dir) != nil {
		return nil, errors.New("opencode: cannot secure state directory")
	}
	path := filepath.Join(dir, "service.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, errors.New("opencode: cannot open private lock")
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		f.Close()
		return nil, errors.New("opencode: unsafe lock file")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) || time.Now().After(deadline) {
			f.Close()
			return nil, errors.New("opencode: another configuration command is busy")
		}
		select {
		case <-c.ctx.Done():
			f.Close()
			return nil, c.ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}
