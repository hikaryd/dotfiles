// Package vpn implements VPN configuration and isolated diagnostics.
package vpn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type object = map[string]any

func str(v any) string { s, _ := v.(string); return s }
func obj(v any) object { o, _ := v.(map[string]any); return o }
func value(m map[string]string, k, d string) string {
	if s, ok := m[k]; ok {
		return s
	}
	return d
}
func env(k, d string) string {
	if s, ok := os.LookupEnv(k); ok {
		return s
	}
	return d
}
func loadConf(path string) (map[string]string, error) {
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if e != nil {
		return nil, errors.New("cannot read VPN configuration")
	}
	m := map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if ok {
			m[strings.TrimSpace(k)] = strings.Trim(strings.Trim(strings.TrimSpace(v), `"`), "'")
		}
	}
	return m, nil
}
func atomicWrite(path string, b []byte, mode os.FileMode) error {
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return errors.New("cannot create destination directory")
	}
	f, e := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".")
	if e != nil {
		return errors.New("cannot create private temporary file")
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(f.Name(), path)
	}
	if e != nil {
		return errors.New("cannot write destination file")
	}
	return nil
}
func jsonBytes(v any) []byte                   { b, _ := json.MarshalIndent(v, "", "  "); return b }
func emit(w io.Writer, event string, m object) { m["event"] = event; _ = json.NewEncoder(w).Encode(m) }
func readURL(ctx context.Context, client *http.Client, url string, max int64, agent string) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
	if e != nil || (req.URL.Scheme != "http" && req.URL.Scheme != "https") {
		return nil, errors.New("source_fetch_failed")
	}
	req.Header.Set("User-Agent", agent)
	r, e := client.Do(req)
	if e != nil {
		return nil, errors.New("source_fetch_failed")
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, errors.New("source_http_status")
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, max+1))
	if e != nil {
		return nil, errors.New("source_fetch_failed")
	}
	if int64(len(b)) > max {
		return nil, errors.New("source_too_large")
	}
	return b, nil
}

// Each external child gets an owned process group; cancellation never signals the caller's group.
type child struct {
	cmd      *exec.Cmd
	done     chan struct{}
	err      error
	stopOnce sync.Once
}

func startChild(cmd *exec.Cmd) (*child, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if e := cmd.Start(); e != nil {
		return nil, errors.New("process unavailable")
	}
	p := &child{cmd: cmd, done: make(chan struct{})}
	go func() { p.err = cmd.Wait(); close(p.done) }()
	return p, nil
}
func (p *child) stop(wait time.Duration) {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() {
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-p.done:
		case <-time.After(wait):
			_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
			<-p.done
		} // Also remove descendants left behind by a completed leader.
		_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
	})
}
func command(ctx context.Context, input string, name string, args ...string) ([]byte, int, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout = &out
	p, e := startChild(cmd)
	if e != nil {
		return nil, -1, e
	}
	select {
	case <-p.done:
		p.stop(100 * time.Millisecond)
		if p.err != nil {
			if x, ok := p.err.(*exec.ExitError); ok {
				return out.Bytes(), x.ExitCode(), nil
			}
			return nil, -1, errors.New("process failed")
		}
		return out.Bytes(), 0, nil
	case <-ctx.Done():
		p.stop(200 * time.Millisecond)
		return nil, -1, ctx.Err()
	}
}
func safeCommand(ctx context.Context, name string, args ...string) (string, error) {
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	b, n, e := command(c, "", name, args...)
	if e != nil {
		return "", fmt.Errorf("Cannot execute %s", filepath.Base(name))
	}
	if n != 0 {
		return "", fmt.Errorf("%s command failed (exit %d)", filepath.Base(name), n)
	}
	return string(b), nil
}

// Run dispatches arguments following the vpn group (or the dots-vpn alias).
func Run(root string, args []string) int { return run(args, os.Stdout, os.Stderr) }
func run(args []string, out, errout io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errout, "usage: dots-vpn {generate|nodes|router-setup|benchmark|xhttp-benchmark|tcp-tuning|awg-params|tuning-env}")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	var e error
	switch args[0] {
	case "generate":
		if len(args) > 1 {
			return 2
		}
		e = generate(ctx, env("VPN_ENV", "/etc/sing-box/vpn.env"), env("VPN_ETC", "/etc/sing-box"), out, errout)
	case "router-setup":
		if len(args) > 1 {
			return 2
		}
		if action := env("ACTION", "install"); action != "install" && action != "identify" {
			fmt.Fprintln(errout, "неизвестный ACTION (install|identify)")
			return 2
		}
		e = routerSetup(ctx, out)
	case "nodes":
		return exportNodes(ctx, args[1:], out, errout)
	case "benchmark":
		return benchmarkMain(ctx, args[1:], out, errout)
	case "xhttp-benchmark":
		return xhttpMain(ctx, args[1:], out, errout)
	case "tcp-tuning":
		return tuningMain(ctx, args[1:], out, errout)
	case "-h", "--help":
		fmt.Fprintln(out, "usage: dots-vpn {generate|nodes|router-setup|benchmark|xhttp-benchmark|tcp-tuning|awg-params|tuning-env}")
		return 0
	default:
		fmt.Fprintln(errout, "unknown VPN command")
		return 2
	}
	if e != nil {
		fmt.Fprintln(errout, e)
		return 1
	}
	return 0
}
