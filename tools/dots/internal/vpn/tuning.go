package vpn

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const tuningService = "awg-tcp-tuning.service"
const tuningParent = "awg-quick@awg0.service"
const routeMetric = 42705
const tuningUnit = `[Unit]
Description=Opt-in TCP BBR for registered AWG peers
BindsTo=awg-quick@awg0.service
PartOf=awg-quick@awg0.service
After=awg-quick@awg0.service

[Service]
Type=oneshot
RemainAfterExit=yes
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ExecStart=/usr/local/libexec/dots-vpn tcp-tuning apply
ExecStop=/usr/local/libexec/dots-vpn tcp-tuning cleanup

[Install]
WantedBy=awg-quick@awg0.service
`

type tuning struct {
	config, helper, unit string
	execute              func(string, ...string) (string, error)
}

func peerCIDR(v any) (string, error) {
	s, ok := v.(string)
	if ok {
		if a, e := netip.ParseAddr(s); e == nil && a.Is4() {
			return a.String() + "/32", nil
		}
		if p, e := netip.ParsePrefix(s); e == nil && p.Addr().Is4() && p.Bits() == 32 {
			return p.String(), nil
		}
	}
	return "", errors.New("Peers must be IPv4 /32 addresses")
}
func (t tuning) readPeers() ([]string, error) {
	b, e := os.ReadFile(t.config)
	if e != nil {
		return nil, errors.New("Cannot read TCP tuning configuration")
	}
	var m object
	if json.Unmarshal(b, &m) != nil {
		return nil, errors.New("Cannot read TCP tuning configuration")
	}
	a, ok := m["peers"].([]any)
	if len(m) != 1 || !ok {
		return nil, errors.New("Configuration must contain only a peers list")
	}
	peers := []string{}
	for _, v := range a {
		p, e := peerCIDR(v)
		if e != nil {
			return nil, e
		}
		if !slices.Contains(peers, p) {
			peers = append(peers, p)
		}
	}
	return peers, nil
}
func (t tuning) validate(peers []string) error {
	if len(peers) == 0 {
		return nil
	}
	s, e := t.execute("awg", "show", "awg0", "allowed-ips")
	if e != nil {
		return e
	}
	allowed := map[string]bool{}
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) > 1 {
			for _, p := range strings.Split(strings.Join(f[1:], ""), ",") {
				allowed[p] = true
			}
		}
	}
	for _, p := range peers {
		if !allowed[p] {
			return errors.New("A requested address is not an actual awg0 peer AllowedIP")
		}
	}
	return nil
}
func (t tuning) routes() ([]object, error) {
	s, e := t.execute("ip", "-j", "-4", "route", "show")
	if e != nil {
		return nil, e
	}
	var data any
	if json.Unmarshal([]byte(s), &data) != nil {
		return nil, errors.New("Cannot decode awg0 routes")
	}
	a, ok := data.([]any)
	if !ok {
		return nil, errors.New("Unexpected awg0 route response")
	}
	r := []object{}
	for _, v := range a {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, errors.New("Unexpected awg0 route entry")
		}
		r = append(r, m)
	}
	return r, nil
}
func destination(r object) string { s, _ := peerCIDR(r["dst"]); return s }
func owned(r object) bool {
	m := obj(r["metrics"])
	if a, ok := r["metrics"].([]any); ok {
		m = object{}
		for _, v := range a {
			for k, v := range obj(v) {
				m[k] = v
			}
		}
	}
	metric := 0
	switch n := r["metric"].(type) {
	case float64:
		if n == routeMetric {
			metric = routeMetric
		}
	case int:
		metric = n
	}
	return destination(r) != "" && r["dev"] == "awg0" && r["protocol"] == "static" && metric == routeMetric && m["congestion"] == "bbr"
}
func (t tuning) delete(p string) error {
	_, e := t.execute("ip", "-4", "route", "del", p, "dev", "awg0", "proto", "static", "metric", strconv.Itoa(routeMetric))
	return e
}
func (t tuning) cleanup() error {
	routes, e := t.routes()
	if e != nil {
		return e
	}
	for _, r := range routes {
		if owned(r) {
			if e = t.delete(destination(r)); e != nil {
				return e
			}
		}
	}
	return nil
}
func (t tuning) apply() error {
	peers, e := t.readPeers()
	if e != nil {
		return e
	}
	if e = t.validate(peers); e != nil {
		return e
	}
	before, e := t.routes()
	if e != nil {
		return e
	}
	existing := map[string]bool{}
	for _, r := range before {
		p := destination(r)
		if slices.Contains(peers, p) && !owned(r) {
			return errors.New("A requested peer already has a route owned by another tool")
		}
		if owned(r) {
			existing[p] = true
		}
	}
	if len(peers) > 0 {
		if _, e = t.execute("modprobe", "tcp_bbr"); e != nil {
			return e
		}
	}
	added := []string{}
	rollback := func(cause error) error {
		for _, p := range added {
			if e := t.delete(p); e != nil {
				cause = errors.Join(cause, fmt.Errorf("route rollback failed: %w", e))
			}
		}
		return cause
	}
	for _, p := range peers {
		if !existing[p] {
			if _, e = t.execute("ip", "-4", "route", "add", p, "dev", "awg0", "proto", "static", "metric", strconv.Itoa(routeMetric), "congctl", "lock", "bbr"); e != nil {
				return rollback(e)
			}
			added = append(added, p)
		}
	}
	for _, p := range peers {
		s, e := t.execute("ip", "-4", "route", "show", "exact", p, "dev", "awg0")
		if e != nil {
			return rollback(e)
		}
		if !strings.Contains(s, "congctl lock bbr") {
			return rollback(errors.New("The requested BBR route was not installed with a lock"))
		}
	}
	for _, r := range before {
		if owned(r) && !slices.Contains(peers, destination(r)) {
			if e = t.delete(destination(r)); e != nil {
				return e
			}
		}
	}
	return nil
}
func (t tuning) save(peers []string) error {
	if peers == nil {
		peers = []string{}
	}
	return atomicWrite(t.config, append(jsonBytes(object{"peers": peers}), '\n'), 0600)
}
func (t tuning) install(peers []string) error {
	if _, e := t.execute("systemctl", "is-active", "--quiet", tuningParent); e != nil {
		return e
	}
	_, statErr := os.Stat(t.config)
	if peers != nil {
		canonical := []string{}
		for _, p := range peers {
			s, e := peerCIDR(p)
			if e != nil {
				return e
			}
			if !slices.Contains(canonical, s) {
				canonical = append(canonical, s)
			}
		}
		peers = canonical
		if e := t.validate(peers); e != nil {
			return e
		}
	} else if statErr == nil {
		p, e := t.readPeers()
		if e != nil {
			return e
		}
		if e = t.validate(p); e != nil {
			return e
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return errors.New("Cannot read TCP tuning configuration")
	}
	self, e := os.Executable()
	if e != nil {
		return errors.New("Cannot locate native executable")
	}
	resolvedSelf, _ := filepath.EvalSymlinks(self)
	resolvedHelper, _ := filepath.EvalSymlinks(t.helper)
	if resolvedSelf != resolvedHelper {
		b, e := os.ReadFile(self)
		if e != nil {
			return errors.New("Cannot read native executable")
		}
		if e = atomicWrite(t.helper, b, 0755); e != nil {
			return e
		}
	}
	if e = atomicWrite(t.unit, []byte(tuningUnit), 0644); e != nil {
		return e
	}
	if peers != nil {
		e = t.save(peers)
	} else if statErr != nil {
		e = t.save(nil)
	} else {
		e = os.Chmod(t.config, 0600)
	}
	if e != nil {
		return e
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", tuningService}, {"restart", tuningService}} {
		if _, e = t.execute("systemctl", args...); e != nil {
			return e
		}
	}
	return nil
}
func (t tuning) disable() error {
	if _, e := t.execute("systemctl", "disable", "--now", tuningService); e != nil {
		return e
	}
	if e := t.cleanup(); e != nil {
		return e
	}
	return t.save(nil)
}
func lockFile(ctx context.Context, path string) (func(), error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, errors.New("cannot open TCP tuning lock")
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if e == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		if e != syscall.EWOULDBLOCK && e != syscall.EAGAIN {
			f.Close()
			return nil, errors.New("cannot acquire TCP tuning lock")
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, errors.New("TCP tuning interrupted")
		case <-timer.C:
			f.Close()
			return nil, errors.New("TCP tuning lock timeout")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

type stringFlags []string

func (s *stringFlags) String() string     { return "" }
func (s *stringFlags) Set(v string) error { *s = append(*s, v); return nil }
func tuningMain(ctx context.Context, args []string, out, errout io.Writer) int {
	fs := flag.NewFlagSet("tcp-tuning", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var peers stringFlags
	fs.Var(&peers, "peer", "explicit IPv4 /32 peer")
	action := ""
	flags := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--peer" && i+1 < len(args) {
			flags = append(flags, a, args[i+1])
			i++
		} else if !strings.HasPrefix(a, "-") && action == "" {
			action = a
		} else {
			flags = append(flags, a)
		}
	}
	e := fs.Parse(flags)
	if errors.Is(e, flag.ErrHelp) {
		fmt.Fprintln(out, "tcp-tuning {install|apply|cleanup|disable} [--peer IPv4/32]")
		return 0
	}
	if e != nil || len(fs.Args()) > 0 || !slices.Contains([]string{"install", "apply", "cleanup", "disable"}, action) {
		fmt.Fprintln(errout, "tcp-tuning: invalid action or arguments")
		return 2
	}
	if peers != nil && action != "install" {
		fmt.Fprintln(errout, "--peer is only valid with install")
		return 2
	}
	if os.Geteuid() != 0 {
		fmt.Fprintln(errout, "root is required")
		return 2
	}
	t := tuning{"/etc/awg-tcp-tuning.json", "/usr/local/libexec/dots-vpn", "/etc/systemd/system/awg-tcp-tuning.service", func(n string, a ...string) (string, error) {
		// Rollback of an owned route still needs its bounded command window after cancellation.
		commandCtx := ctx
		if n == "ip" && len(a) > 2 && a[2] == "del" {
			commandCtx = context.WithoutCancel(ctx)
		}
		return safeCommand(commandCtx, n, a...)
	}}
	unlock, e := lockFile(ctx, "/run/lock/awg-tcp-tuning.lock")
	if e == nil {
		if action == "install" || action == "disable" {
			unlock()
		} else {
			defer unlock()
		}
		switch action {
		case "install":
			e = t.install(peers)
		case "disable":
			e = t.disable()
		case "apply":
			e = t.apply()
		case "cleanup":
			e = t.cleanup()
		}
	}
	if e != nil {
		fmt.Fprintf(errout, "TCP tuning failed: %s\n", e)
		return 1
	}
	fmt.Fprintf(out, "TCP tuning: %s completed\n", action)
	return 0
}
