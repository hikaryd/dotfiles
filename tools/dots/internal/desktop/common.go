package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const maxBytes = 1024 * 1024

func home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return u.HomeDir
	}
	// A missing account directory must never turn private paths into cwd paths.
	return "/nonexistent"
}
func envOr(key, fallback string) string {
	if s := os.Getenv(key); s != "" {
		return s
	}
	return fallback
}
func configHome() string { return envOr("XDG_CONFIG_HOME", filepath.Join(home(), ".config")) }
func expand(s string) string {
	if s == "~" {
		return home()
	}
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(home(), s[2:])
	}
	return s
}
func resolve(s string) string {
	s, _ = filepath.Abs(expand(s))
	if p, e := filepath.EvalSymlinks(s); e == nil {
		return p
	}
	parent := filepath.Dir(s)
	if parent != s {
		return filepath.Join(resolve(parent), filepath.Base(s))
	}
	return s
}
func isDir(p string) bool  { s, e := os.Stat(p); return e == nil && s.IsDir() }
func isFile(p string) bool { s, e := os.Stat(p); return e == nil && s.Mode().IsRegular() }
func exists(p string) bool { _, e := os.Lstat(p); return e == nil }
func within(p, root string) bool {
	r, e := filepath.Rel(root, p)
	return e == nil && r != ".." && !strings.HasPrefix(r, ".."+string(os.PathSeparator))
}
func has(s string, list ...string) bool {
	for _, v := range list {
		if s == v {
			return true
		}
	}
	return false
}
func cleanText(s string) bool {
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool { return r < 32 || r == 127 })
}
func control(r rune) bool {
	// unicode.C omits unassigned code points; Python's category C includes them.
	return !unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Z)
}
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return '?'
		}
		return r
	}, s)
}
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
func available(name string) bool { _, e := exec.LookPath(name); return e == nil }
func jsonBytes(v any, pretty bool) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if pretty {
		e.SetIndent("", "  ")
	}
	err := e.Encode(v)
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), err
}
func printJSON(v any, pretty bool) error {
	b, e := jsonBytes(v, pretty)
	if e == nil {
		_, e = fmt.Fprintln(os.Stdout, string(b))
	}
	return e
}
func atomicWrite(p string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+"-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), p)
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	left := b.limit - b.Len()
	if n > left {
		b.overflow = true
		p = p[:max(0, left)]
	}
	b.Buffer.Write(p)
	return n, nil
}

type commandResult struct {
	out, err string
	code     int
}

// Captured errors never contain argv or stderr, which may carry credentials.
func captureCommand(timeout time.Duration, limit int, input string, env []string, args ...string) (commandResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.WaitDelay = time.Second
	out := &limitedBuffer{limit: limit}
	er := &limitedBuffer{limit: 65536}
	cmd.Stdout = out
	cmd.Stderr = er
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	if env != nil {
		cmd.Env = env
	}
	e := cmd.Run()
	r := commandResult{out: out.String(), err: er.String()}
	if out.overflow {
		return r, errors.New("Превышен лимит вывода команды.")
	}
	if ctx.Err() != nil {
		return r, errors.New("внешняя команда превысила таймаут")
	}
	if e != nil {
		var ee *exec.ExitError
		if errors.As(e, &ee) {
			r.code = ee.ExitCode()
			return r, nil
		}
		return r, errors.New("внешняя команда недоступна")
	}
	return r, nil
}
func command(args ...string) (string, error) {
	r, e := captureCommand(5*time.Second, 16*maxBytes, "", nil, args...)
	if e != nil {
		return "", e
	}
	if r.code != 0 {
		return "", errors.New("внешняя команда завершилась с ошибкой")
	}
	return r.out, nil
}
func interactive(args ...string) int {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	e := cmd.Run()
	if e == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(e, &ee) {
		if ee.ExitCode() < 0 {
			return 130
		}
		return ee.ExitCode()
	}
	fmt.Fprintf(os.Stderr, "Не найдена команда: %s\n", args[0])
	return 127
}
func envSet(env []string, kv ...string) []string {
	out := append([]string{}, env...)
	for _, v := range kv {
		k := strings.SplitN(v, "=", 2)[0] + "="
		filtered := out[:0]
		for _, old := range out {
			if !strings.HasPrefix(old, k) {
				filtered = append(filtered, old)
			}
		}
		out = append(filtered, v)
	}
	return out
}
func readLimited(r io.Reader, limit int) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, int64(limit+1)))
	if len(b) > limit {
		return nil, errors.New("Лимит 1 MiB превышен.")
	}
	return b, e
}

type options struct {
	values map[string]string
	pos    []string
}

func parse(args []string, bools, values string) (options, error) {
	o := options{values: map[string]string{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			o.pos = append(o.pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			o.pos = append(o.pos, a)
			continue
		}
		k, v, eq := strings.Cut(a, "=")
		if has(k, strings.Fields(bools)...) {
			if eq {
				return o, errors.New("флаг не принимает значение")
			}
			o.values[k] = "true"
		} else if has(k, strings.Fields(values)...) {
			if !eq {
				i++
				if i >= len(args) {
					return o, errors.New("пропущено значение аргумента")
				}
				v = args[i]
			}
			o.values[k] = v
		} else {
			return o, errors.New("неизвестный аргумент")
		}
	}
	return o, nil
}
func (o options) flag(s string) bool { return o.values[s] == "true" }
func usageError(command string) int {
	fmt.Fprintln(os.Stderr, "dots "+command+": некорректные аргументы; используйте --help")
	return 2
}
func fail(prefix string, e error) int { fmt.Fprintf(os.Stderr, "%s: %s\n", prefix, e); return 1 }
func selectLine(lines []string, gum, fzf []string, safe bool) (int, error) {
	gum, fzf = selectorArgs(gum, fzf, colorEnabled(os.Stderr))
	var argv []string
	if available("fzf") {
		argv = append([]string{"fzf"}, fzf...)
	} else if available("gum") {
		argv = append([]string{"gum"}, gum...)
	} else {
		return -1, errors.New("Нужен gum или fzf для выбора.")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	cmd.Stderr = os.Stderr
	out := &limitedBuffer{limit: maxBytes}
	cmd.Stdout = out
	if safe {
		cmd.Env = envSet(os.Environ(), "FZF_DEFAULT_OPTS=", "FZF_DEFAULT_OPTS_FILE=/dev/null")
	}
	if e := cmd.Run(); e != nil {
		return -1, nil
	}
	if out.overflow {
		return -1, errors.New("Превышен лимит вывода выбора.")
	}
	answer := strings.TrimRight(out.String(), "\n")
	for i, line := range lines {
		if line == answer {
			return i, nil
		}
	}
	return -1, nil
}
