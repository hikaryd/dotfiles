// Package metrics implements a read-only Prometheus/VictoriaMetrics dashboard.
// Connection coordinates belong to a private local inventory, never the binary.
package metrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"
)

type inventory struct {
	Profiles  []profile         `json:"profiles"`
	Shortcuts map[string]string `json:"shortcuts,omitempty"` // command → private profile name
}

type profile struct {
	Name                string            `json:"name"`
	URL                 string            `json:"url,omitempty"`
	Forward             *forward          `json:"port_forward,omitempty"`
	BearerFile          string            `json:"bearer_token_file,omitempty"`
	Namespace           string            `json:"namespace"`
	Service             string            `json:"service,omitempty"`
	RequestMetric       string            `json:"request_metric,omitempty"`
	DurationMetric      string            `json:"duration_metric,omitempty"`
	RouteDurationMetric string            `json:"route_duration_metric,omitempty"`
	StatusLabel         string            `json:"status_label,omitempty"`
	RouteLabel          string            `json:"route_label,omitempty"`
	RouteFilter         string            `json:"route_filter,omitempty"`
	SectionNotes        map[string]string `json:"section_notes,omitempty"`
	CustomOnly          bool              `json:"custom_only,omitempty"`
	Queries             []metric          `json:"queries,omitempty"`
}

type forward struct {
	Kubeconfig string `json:"kubeconfig"`
	Context    string `json:"context"`
	Namespace  string `json:"namespace"`
	Resource   string `json:"resource"`
	RemotePort int    `json:"remote_port"`
}

type metric struct {
	// Derived queries may pin a ranking to the exact batch end, including on
	// servers that align query_range boundaries to their cache step.
	queryAt        func(string) string
	Histogram      *histogramSpec `json:"histogram,omitempty"`
	ServiceQuery   string         `json:"service_query,omitempty"`
	ServicePattern string         `json:"service_pattern,omitempty"`
	NamespaceOnly  bool           `json:"namespace_only,omitempty"`
	Section        string         `json:"section,omitempty"`
	Scope          string         `json:"scope,omitempty"`
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Unit           string         `json:"unit"`
	Query          string         `json:"query"`
	Description    string         `json:"description,omitempty"`
}

var identifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var metricName = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
var profileName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
var resourceName = regexp.MustCompile(`^(svc|service|pod)/[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)

func configPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	path := filepath.Join(base, "dots-kube", "metrics.json")
	if outsideGit(path) == nil {
		return path
	}
	// Some dotfile managers make ~/.config a working tree. Do not silently
	// weaken privacy there; choose a data directory outside the checkout.
	base = os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "dots-private", "metrics.json")
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// Check the lexical and resolved ancestors, including symlinked directories and
// Git worktrees (.git can be a file). Even ignored files in checkouts are refused.
func outsideGit(path string) error {
	abs, err := filepath.Abs(expandPath(path))
	if err != nil {
		return errors.New("cannot resolve private file location")
	}
	check := func(dir string) error {
		for {
			if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil || !os.IsNotExist(err) {
				return errors.New("private files must be outside Git working trees")
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return nil
			}
			dir = parent
		}
	}
	if err := check(filepath.Dir(abs)); err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		if err := check(filepath.Dir(resolved)); err != nil {
			return err
		}
	}
	dir := filepath.Dir(abs)
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			return check(resolved)
		}
		if !os.IsNotExist(err) || filepath.Dir(dir) == dir {
			return errors.New("cannot resolve private directory")
		}
		dir = filepath.Dir(dir)
	}
}

func readPrivate(path string, limit int64) ([]byte, error) {
	path = expandPath(path)
	if err := outsideGit(path); err != nil {
		return nil, err
	}
	// O_NOFOLLOW prevents reading a swapped symlink; fstat validates the opened
	// file, not a pathname checked before opening it.
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.New("cannot open private file (missing, inaccessible or symlink)")
	}
	file := os.NewFile(uintptr(fd), "private")
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || (info.Mode().Perm() != 0600 && info.Mode().Perm() != 0400) {
		return nil, errors.New("private file must be regular with permissions 0600 or 0400")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("private file unreadable or too large")
	}
	return data, nil
}

func initConfig(path string) error {
	path = expandPath(path)
	if err := outsideGit(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return errors.New("cannot create private config directory")
	}
	// Empty coordinates are deliberate: no cluster/endpoint/port baked in.
	template := inventory{Profiles: []profile{{Name: "default", Namespace: "", Forward: &forward{}}}}
	data, _ := json.MarshalIndent(template, "", "  ")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("cannot create private config; existing files are never overwritten")
	}
	_, writeErr := file.Write(append(data, '\n'))
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("cannot persist private config")
	}
	return nil
}

func loadConfig(path string) (inventory, error) {
	var cfg inventory
	data, err := readPrivate(path, 256*1024)
	if err != nil {
		return cfg, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, errors.New("invalid metrics JSON (check fields and syntax locally)")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return cfg, errors.New("metrics config must contain one JSON object")
	}
	if len(cfg.Profiles) == 0 || len(cfg.Profiles) > 32 {
		return cfg, errors.New("configure between 1 and 32 profiles")
	}
	seen := map[string]bool{}
	for i := range cfg.Profiles {
		p := &cfg.Profiles[i]
		if err := p.validate(); err != nil {
			return cfg, fmt.Errorf("profile #%d: %w", i+1, err)
		}
		if seen[p.Name] {
			return cfg, errors.New("profile names must be unique")
		}
		seen[p.Name] = true
	}
	for name, profile := range cfg.Shortcuts {
		if (name != "km" && name != "kma") || !seen[profile] {
			return cfg, errors.New("shortcuts must map km/kma to existing private profile names")
		}
	}
	return cfg, nil
}

func (p *profile) validate() error {
	if !profileName.MatchString(p.Name) {
		return errors.New("name must be a short alphanumeric identifier")
	}
	if (p.URL == "") == (p.Forward == nil) {
		return errors.New("configure exactly one of url or port_forward")
	}
	if p.URL != "" {
		u, err := url.Parse(p.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("url must be HTTP(S), without credentials, query or fragment")
		}
		ip := net.ParseIP(u.Hostname())
		if u.Scheme == "http" && u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return errors.New("non-loopback URLs require HTTPS")
		}
	}
	if f := p.Forward; f != nil {
		if f.Kubeconfig == "" || f.Context == "" || strings.HasPrefix(f.Context, "-") || !dnsLabel.MatchString(f.Namespace) || len(f.Namespace) > 63 || !resourceName.MatchString(f.Resource) || f.RemotePort < 1 || f.RemotePort > 65535 {
			return errors.New("port_forward requires kubeconfig, context, namespace, resource (svc/name or pod/name), remote_port")
		}
		if err := outsideGit(f.Kubeconfig); err != nil {
			return err
		}
	}
	if !p.CustomOnly && (!dnsLabel.MatchString(p.Namespace) || len(p.Namespace) > 63) {
		return errors.New("namespace is required for built-in metrics")
	}
	if p.RequestMetric == "" {
		p.RequestMetric = "http_requests_total"
	}
	if p.DurationMetric == "" {
		p.DurationMetric = "http_request_duration_seconds"
	}
	if p.StatusLabel == "" {
		p.StatusLabel = "status"
	}
	if p.RouteDurationMetric == "" {
		p.RouteDurationMetric = p.DurationMetric
	}
	if !metricName.MatchString(p.RouteDurationMetric) {
		return errors.New("invalid route_duration_metric")
	}
	if !metricName.MatchString(p.RequestMetric) || !metricName.MatchString(p.DurationMetric) || !identifier.MatchString(p.StatusLabel) || p.StatusLabel == "namespace" || p.StatusLabel == "service" {
		return errors.New("invalid metric name or status_label")
	}
	if p.RouteLabel != "" && (!identifier.MatchString(p.RouteLabel) || p.RouteLabel == "namespace" || p.RouteLabel == "service" || p.RouteLabel == "method" || p.RouteLabel == "le" || p.RouteLabel == p.StatusLabel || p.RouteLabel == "__name__") {
		return errors.New("route_label must name a route template label, not a reserved dimension")
	}
	if err := validateRouteFilter(p.RouteFilter); err != nil {
		return err
	}
	for id, note := range p.SectionNotes {
		if _, ok := sectionIndex(id); !ok || len(note) > 2048 {
			return errors.New("section_notes require known section IDs and at most 2048 bytes per note")
		}
	}
	if len(p.Queries) > 96 || (p.CustomOnly && len(p.Queries) == 0) {
		return errors.New("custom_only requires queries; at most 96 custom queries")
	}
	seen := map[string]bool{}
	if !p.CustomOnly {
		for _, m := range builtins(*p) {
			seen[m.ID] = true
		}
		// Reserve optional IDs even before their instrumentation is configured.
		for _, s := range sections {
			for _, id := range s.Metrics {
				seen[id] = true
			}
		}
	}
	for _, m := range p.Queries {
		if err := m.validateService(); err != nil {
			return err
		}
		if err := m.Histogram.validate(); err != nil {
			return err
		}
		if _, ok := sectionIndex(querySection(m)); !ok || len(m.Scope) > 300 {
			return errors.New("queries require a known section and a scope of at most 300 bytes")
		}
		if !profileName.MatchString(m.ID) || m.Title == "" || len(m.Title) > 200 || len(m.Query) == 0 || len(m.Query) > 8192 || len(m.Unit) > 32 || len(m.Description) > 1024 || seen[m.ID] {
			return errors.New("queries require unique id, title, query and bounded text fields")
		}
		seen[m.ID] = true
	}
	return nil
}

func (p profile) metrics() []metric {
	var result []metric
	if !p.CustomOnly {
		result = append(builtins(p), routeMetrics(p)...)
	}
	for _, m := range p.Queries {
		if p.Service != "" && m.NamespaceOnly {
			continue
		}
		result = append(result, m.forService(p.Service))
	}
	return result
}
