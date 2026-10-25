package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const healthInterval = 30 * time.Second
const readinessLifetime = 90 * time.Second

type monitorState struct{ failures int }

var errNotReady = errors.New("Proxy is not ready. Install/check the background service: dots-proxy --install-service; dots-proxy --status")

// Read-only identity: unlike process-control helpers, this never repairs PID files.
func (c *controller) liveRoute() (int, string, error) {
	data, err := os.ReadFile(c.path("xray.pid"))
	if err != nil {
		return 0, "", errNotReady
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || !pidRunning(pid) || !c.owned(pid) {
		return 0, "", errNotReady
	}
	data, err = os.ReadFile(c.path("config.json"))
	var cfg object
	var listener struct {
		Inbounds []struct {
			Port             int
			Listen, Protocol string
		}
	}
	if err != nil || json.Unmarshal(data, &cfg) != nil || json.Unmarshal(data, &listener) != nil {
		return 0, "", errNotReady
	}
	matches := false
	for _, in := range listener.Inbounds {
		if in.Port == c.port && in.Listen == "127.0.0.1" && in.Protocol == "http" {
			matches = true
		}
	}
	if !matches {
		return 0, "", errNotReady
	}
	return pid, digest(cfg), nil
}

func (c *controller) requireReady() error {
	pid, key, err := c.liveRoute()
	if err != nil {
		return errNotReady
	}
	data, err := os.ReadFile(c.path("selection.json"))
	var s selection
	now := time.Now()
	if err != nil || json.Unmarshal(data, &s) != nil || s.Version != 1 || s.PID != pid || s.Server != key || !allowedCountry(s.Country) || s.Latency <= 0 || now.Before(s.Checked) || now.Sub(s.Checked) > readinessLifetime {
		return errNotReady
	}
	if err := c.listenerOwned(pid); err != nil {
		return errNotReady
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.port)), 250*time.Millisecond)
	if err != nil {
		return errNotReady
	}
	conn.Close()
	nextPID, nextKey, err := c.liveRoute()
	if err != nil || nextPID != pid || nextKey != key {
		return errNotReady
	}
	return nil
}

var errListenerMissing = errors.New("Proxy listener is absent.")

// Socket ownership cannot be inferred from the PID file or the config alone.
func (c *controller) listenerOwned(pid int) error {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP:"+strconv.Itoa(c.port), "-sTCP:LISTEN", "-Fpn").Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 && len(output) == 0 && ctx.Err() == nil {
			return errListenerMissing
		}
		return errors.New("Could not verify proxy listener ownership.")
	}
	found := false
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "p") && line != "p"+strconv.Itoa(pid) {
			return errors.New("Proxy listener belongs to another process.")
		}
		if strings.HasPrefix(line, "n") {
			if line != "n127.0.0.1:"+strconv.Itoa(c.port) {
				return errors.New("Proxy listener address does not match its config.")
			}
			found = true
		}
	}
	if !found {
		return errListenerMissing
	}
	return nil
}

// Refuse foreign state before selection can change config or signal anything.
func (c *controller) noForeignListener() error {
	data, _ := os.ReadFile(c.path("xray.pid"))
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if pidRunning(pid) {
		if !c.owned(pid) {
			return errors.New("Refusing foreign process in proxy PID file.")
		}
		err := c.listenerOwned(pid)
		if errors.Is(err, errListenerMissing) {
			return nil
		}
		return err
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.port)))
	if err != nil {
		return errors.New("Proxy port is occupied; refusing to replace an unowned listener.")
	}
	return listener.Close()
}

func (c *controller) withdrawReadiness() error {
	data, err := os.ReadFile(c.path("selection.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	var s selection
	if err != nil {
		return errors.New("Could not withdraw proxy readiness.")
	}
	if json.Unmarshal(data, &s) != nil {
		// A malformed cache cannot be a useful recovery hint or readiness proof.
		return os.Remove(c.path("selection.json"))
	}
	// Retain the last route as a recovery hint, but it no longer certifies a PID.
	s.PID = 0
	return writeJSON(c.path("selection.json"), s)
}

// Cache I/O must not be able to keep a confirmed unsafe owned child alive.
func (c *controller) rejectRU() error {
	withdrawErr := c.withdrawReadiness()
	if err := c.stop(); err != nil {
		return err
	}
	return withdrawErr
}

func (c *controller) maintain(state *monitorState) error {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	unlock, err := c.lockFile("selection.lock", 5*time.Second)
	if err != nil {
		return err
	}
	recoverRoute := false
	err = func() error {
		defer unlock()
		if err := c.noForeignListener(); err != nil {
			return err
		}
		pid, key, liveErr := c.liveRoute()
		if liveErr != nil {
			// An owned process with unreadable/mismatched config is not safe to adopt.
			if c.requireRunning() == nil {
				return errNotReady
			}
			if err := c.withdrawReadiness(); err != nil {
				return err
			}
			recoverRoute = true
			return nil
		}
		m, checkErr := c.measureRoute(ctx, c.proxyURL(), 1)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		nextPID, nextKey, identityErr := c.liveRoute()
		if identityErr != nil || pid != nextPID || key != nextKey {
			return errNotReady
		}
		if checkErr == nil && allowedCountry(m.Country) && m.Latency > 0 {
			if err := c.saveSelection([]string{key}, 0, m); err != nil {
				return err
			}
			state.failures = 0
			return nil
		}
		state.failures++
		if m.Country != "RU" && state.failures < 3 {
			return errors.New("Proxy health check failed; keeping the existing process during the grace period.")
		}
		if m.Country == "RU" {
			if err := c.rejectRU(); err != nil {
				return err
			}
		} else if err := c.withdrawReadiness(); err != nil {
			return err
		}
		recoverRoute = true
		return nil
	}()
	if err != nil || !recoverRoute {
		return err
	}
	// autoStart owns selection.lock itself and preserves the old process until a
	// temporary candidate has passed its country/OpenAI check.
	if err := c.autoStart(false); err != nil {
		return err
	}
	state.failures = 0
	return nil
}

func (c *controller) daemon() error {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	unlock, err := c.lockFile("supervisor.lock", 0)
	if err != nil {
		return errors.New("A proxy supervisor is already running or its lock is unavailable.")
	}
	defer unlock()
	state := &monitorState{}
	for {
		if err := c.maintain(state); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Deliberately do not print arbitrary errors (subscriptions/credentials).
			fmt.Fprintln(c.errOut, "Proxy supervisor: health/recovery failed; retrying in 30 seconds.")
		}
		timer := time.NewTimer(healthInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
