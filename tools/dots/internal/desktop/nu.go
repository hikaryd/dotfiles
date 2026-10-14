package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

func privateDir(p string) error {
	info, e := os.Lstat(p)
	if e == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Каталог команд не должен быть символической ссылкой")
	}
	if e = os.MkdirAll(p, 0700); e != nil {
		return e
	}
	return os.Chmod(p, 0700)
}
func initAtuin(env []string) error {
	if !available("atuin") {
		return nil
	}
	base := configHome()
	if runtime.GOOS == "darwin" {
		base = filepath.Join(home(), "Library/Application Support")
	}
	autoload := filepath.Join(base, "nushell/autoload")
	target := filepath.Join(autoload, "80-dots-atuin.nu")
	if exists(target) {
		return nil
	}
	r, e := captureCommand(5*time.Second, 4*maxBytes, "", env, "atuin", "init", "nu", "--disable-up-arrow", "--disable-ai")
	if e != nil {
		return e
	}
	if r.code != 0 || r.out == "" {
		return nil
	}
	if e = os.MkdirAll(autoload, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(autoload, ".atuin-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	_, e = f.WriteString("# Создан dots-nu; удалить для повторной генерации.\n" + r.out)
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	e = os.Link(f.Name(), target)
	if os.IsExist(e) {
		return nil
	}
	return e
}
func uniqueStrings(a []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
func prepareNu(root string, args []string) (string, []string, []string, error) {
	root = resolve(envOr("DOTS_ROOT", root))
	parts := append([]string{filepath.Join(home(), ".local/bin"), "/opt/homebrew/bin", "/opt/homebrew/sbin"}, strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))...)
	path := strings.Join(uniqueStrings(parts), string(os.PathListSeparator))
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", path)
	defer os.Setenv("PATH", oldPath)
	nu, e := exec.LookPath("nu")
	if e != nil {
		return "", nil, nil, errors.New("Nushell не найден в PATH")
	}
	cache := filepath.Join(envOr("XDG_CACHE_HOME", filepath.Join(home(), ".cache")), "dots/zsh-bridge")
	if e = privateDir(cache); e != nil {
		return "", nil, nil, e
	}
	bin := filepath.Join(cache, "bin")
	if e = privateDir(bin); e != nil {
		return "", nil, nil, e
	}
	bridge := filepath.Join(root, "scripts/dots-zsh")
	exe, e := os.Executable()
	if e != nil {
		return "", nil, nil, e
	}
	inputs := []string{filepath.Join(root, "config/zsh/commands.zsh"), filepath.Join(root, "config/zsh/kube-tools.zsh"), filepath.Join(root, "config/shell/pi-proxy.sh"), exe, bridge, envOr("DOTS_PRIVATE_ZSH", filepath.Join(configHome(), "dots/private.zsh"))}
	functions, e := filepath.Glob(filepath.Join(root, "config/zsh/functions/*"))
	if e != nil {
		return "", nil, nil, e
	}
	inputs = append(inputs, functions...)
	signature := [][]any{}
	for _, p := range inputs {
		info, e := os.Stat(p)
		if os.IsNotExist(e) {
			signature = append(signature, []any{p, nil, nil})
		} else if e != nil {
			return "", nil, nil, e
		} else {
			signature = append(signature, []any{p, info.ModTime().UnixNano(), info.Size()})
		}
	}
	sig, _ := json.Marshal(signature)
	var state struct {
		Signature json.RawMessage `json:"signature"`
	}
	statePath := filepath.Join(cache, "commands.json")
	b, _ := os.ReadFile(statePath)
	_ = json.Unmarshal(b, &state)
	var compact bytes.Buffer
	if state.Signature != nil {
		_ = json.Compact(&compact, state.Signature)
	}
	env := envSet(os.Environ(), "PATH="+path, "DOTS_ROOT="+root, "DOTS_ZSH_BIN="+bin)
	if !bytes.Equal(sig, compact.Bytes()) {
		r, e := captureCommand(10*time.Second, 4*maxBytes, "", env, bridge, "--list")
		if e != nil || r.code != 0 {
			return "", nil, nil, errors.New("Не удалось получить команды zsh")
		}
		names := map[string]bool{}
		for _, name := range strings.Split(r.out, "\n") {
			if regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`).MatchString(name) && !has(name, "dots-zsh", "dots-nu", "vs") {
				names[name] = true
			}
		}
		entries, e := os.ReadDir(bin)
		if e != nil {
			return "", nil, nil, e
		}
		for _, entry := range entries {
			p := filepath.Join(bin, entry.Name())
			if entry.Type()&os.ModeSymlink != 0 && resolve(p) == resolve(bridge) && !names[entry.Name()] {
				if e = os.Remove(p); e != nil {
					return "", nil, nil, e
				}
			}
		}
		sorted := []string{}
		for name := range names {
			sorted = append(sorted, name)
		}
		sort.Strings(sorted)
		for _, name := range sorted {
			p := filepath.Join(bin, name)
			if !exists(p) {
				if e = os.Symlink(bridge, p); e != nil {
					return "", nil, nil, e
				}
			}
		}
		b, e = json.Marshal(map[string]any{"signature": signature})
		if e != nil {
			return "", nil, nil, e
		}
		if e = atomicWrite(statePath, b); e != nil {
			return "", nil, nil, e
		}
	}
	parts = []string{bin}
	for _, p := range strings.Split(path, string(os.PathListSeparator)) {
		if p != bin {
			parts = append(parts, p)
		}
	}
	env = envSet(env, "PATH="+strings.Join(parts, string(os.PathListSeparator)))
	if e = initAtuin(env); e != nil {
		return "", nil, nil, e
	}
	argv := append([]string{nu, "--config", filepath.Join(root, "config/nushell/config.nu")}, args...)
	return nu, argv, env, nil
}
func nuRun(root string, args []string) int {
	nu, argv, env, e := prepareNu(root, args)
	if e != nil {
		return fail("dots-nu", errors.New("запуск Nushell или подготовка каталога команд не удались"))
	}
	if e = syscall.Exec(nu, argv, env); e != nil {
		return fail("dots-nu", errors.New("не удалось запустить Nushell"))
	}
	return 0
}
