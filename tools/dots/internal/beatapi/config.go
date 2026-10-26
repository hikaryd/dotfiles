// Package beatapi provides a single-user loopback API for BeatAPI models.
package beatapi

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode"
)

const defaultPort = 6446
const maxConfig = 64 << 10 // Covers JSON escaping of an accepted 8 KiB key.
const label = "local.dots.beatapi-proxy"
const jevModel = "jev-1.14"
const legacyJevModel = "jev-1.14-free"

// Config is private on disk; never format it in diagnostics.
type Config struct {
	APIKey   string `json:"api_key"`
	Port     int    `json:"port"`
	ProxyURL string `json:"proxy_url"`
}

var errConfig = errors.New("beatapi: cannot read secure configuration; run setup or key")

func (c Config) validate() error {
	if c.APIKey == "" || c.APIKey == "public" || len(c.APIKey) > 8192 || strings.IndexFunc(c.APIKey, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return errors.New("beatapi: provide a non-public API key without whitespace (maximum 8 KiB)")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("beatapi: local port must be between 1 and 65535")
	}
	u, err := url.Parse(c.ProxyURL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("beatapi: outgoing proxy must be http://127.0.0.1:PORT")
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil || p < 1 || p > 65535 {
		return errors.New("beatapi: invalid outgoing proxy port")
	}
	if p == c.Port {
		return errors.New("beatapi: API and outgoing proxy ports must differ")
	}
	return nil
}
func (c Config) address() string { return net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Port)) }

// checkPath trusts the deliberately configured HOME/XDG base, but rejects
// symlinks in every descendant component, not only the final file. macOS also
// owns /var and /tmp aliases; resolving those permits ordinary temporary paths.
func checkPath(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	trusted := []string{home, base}
	if runtime.GOOS == "darwin" {
		trusted = append(trusted, "/var", "/tmp")
	}
	root := string(filepath.Separator)
	for _, candidate := range trusted {
		if !filepath.IsAbs(candidate) {
			continue
		}
		candidate = filepath.Clean(candidate)
		rel, err := filepath.Rel(candidate, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && len(candidate) > len(root) {
			root = candidate
		}
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	current, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		st, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return errors.New("beatapi: refusing symbolic link")
		}
	}
	return nil
}
func privateDir(path string) error {
	if err := checkPath(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
	}
	st, err := os.Lstat(path)
	if err != nil || !st.IsDir() {
		return errors.New("beatapi: invalid private directory")
	}
	return os.Chmod(path, 0700)
}
func privateFile(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return errConfig
	}
	return nil
}
func loadConfig(path string) (Config, error) {
	var c Config
	if checkPath(filepath.Dir(path)) != nil || privateFile(path) != nil {
		return c, errConfig
	}
	st, err := os.Stat(filepath.Dir(path))
	if err != nil || st.Mode().Perm()&0077 != 0 {
		return c, errConfig
	}
	f, err := os.Open(path)
	if err != nil {
		return c, errConfig
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxConfig+1))
	if err != nil || len(b) > maxConfig || json.Unmarshal(b, &c) != nil || c.validate() != nil {
		return Config{}, errConfig
	}
	return c, nil
}
func saveConfig(path string, c Config) error {
	if err := c.validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return errors.New("beatapi: cannot encode configuration")
	}
	if err := privateDir(filepath.Dir(path)); err != nil {
		return errors.New("beatapi: cannot secure configuration directory")
	}
	if err := atomicPrivate(path, append(b, '\n')); err != nil {
		return errors.New("beatapi: cannot save configuration safely")
	}
	return nil
}
func atomicPrivate(path string, b []byte) error {
	if err := checkPath(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if st, err := os.Lstat(path); err == nil && !st.Mode().IsRegular() {
		return errors.New("beatapi: not a regular file")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".beatapi-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
