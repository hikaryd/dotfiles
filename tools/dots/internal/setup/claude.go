package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var placeholder = regexp.MustCompile(`\$\{(\w+)\}`)
var secretKey = regexp.MustCompile(`(?i)(KEY|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIAL)`)

func substitute(v any) any {
	switch x := v.(type) {
	case string:
		return placeholder.ReplaceAllStringFunc(x, func(s string) string {
			n := s[2 : len(s)-1]
			switch n {
			case "ANTHROPIC_BASE_URL", "Z_AI_API_KEY", "MAGIC_API_KEY", "KUBECONFIG_PATH":
				if val := os.Getenv(n); val != "" {
					return val
				}
			}
			return s
		})
	case []any:
		r := make([]any, len(x))
		for i, v := range x {
			r[i] = substitute(v)
		}
		return r
	case map[string]any:
		r := map[string]any{}
		for k, v := range x {
			r[k] = substitute(v)
		}
		return r
	default:
		return v
	}
}
func cliQuiet(binary string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.WaitDelay = time.Second
	if e := cmd.Run(); e != nil {
		return errors.New("external command failed")
	}
	return nil
}
func orderedKeys(p string) ([]string, error) {
	b, e := os.ReadFile(p)
	if e != nil {
		return nil, errors.New("cannot read JSON file")
	}
	return keysOfObject(b)
}
func claudeApply(root string) error {
	dir := envDefault([]string{"CLAUDE_DIR"}, envDefault([]string{"CLAUDE_CONFIG_DIR"}, filepath.Join(home(), ".claude")))
	src := envDefault([]string{"SRC"}, filepath.Join(root, "config/claude"))
	omc, e := os.ReadFile(filepath.Join(src, "omc-config.json"))
	if e != nil {
		return errors.New("cannot read omc-config.json")
	}
	if _, e = object(omc); e != nil {
		return errors.New("invalid omc-config.json")
	}
	settings, e := readObject(filepath.Join(src, "settings.json"), false)
	if e != nil {
		return e
	}
	servers, e := readObject(filepath.Join(src, "mcp-servers.json"), false)
	if e != nil {
		return e
	}
	names, e := orderedKeys(filepath.Join(src, "mcp-servers.json"))
	if e != nil {
		return e
	}
	env := map[string]any{}
	if v, ok := settings["env"]; ok {
		var yes bool
		env, yes = v.(map[string]any)
		if !yes {
			return errors.New("invalid settings env")
		}
	}
	msg := ""
	if _, ok := env["ANTHROPIC_BASE_URL"]; ok {
		if v := os.Getenv("ANTHROPIC_BASE_URL"); v != "" {
			env["ANTHROPIC_BASE_URL"] = v
			msg = "  settings.json: ANTHROPIC_BASE_URL подставлен из секрета"
		} else {
			delete(env, "ANTHROPIC_BASE_URL")
			msg = "  settings.json: ANTHROPIC_BASE_URL не задан — ключ убран (дефолтный endpoint)"
		}
	}
	var changes []change
	for _, v := range []struct {
		name string
		b    []byte
	}{{".omc-config.json", omc}, {"settings.json", encode(settings)}} {
		p := filepath.Join(dir, v.name)
		target, e := resolve(p)
		if e != nil {
			return e
		}
		changes = append(changes, change{path: p, data: v.b, follow: true, backup: target + ".bak-" + stamp(), backupPrivate: true})
	}
	if e = commit(changes); e != nil {
		return e
	}
	fmt.Println("  .omc-config.json: записан")
	if msg != "" {
		fmt.Println(msg)
	}
	fmt.Println("  settings.json: записан")
	if os.Getenv("HAVE_CLAUDE") != "1" {
		fmt.Println("  ⚠ команда 'claude' не найдена в PATH — регистрация MCP пропущена")
		return nil
	}
	var registered []string
	type skip struct {
		name string
		why  []string
	}
	var skipped []skip
	for _, name := range names {
		resolved := substitute(servers[name])
		b := encode(resolved)
		matches := placeholder.FindAllStringSubmatch(string(b), -1)
		if len(matches) > 0 {
			set := map[string]bool{}
			for _, m := range matches {
				set[m[1]] = true
			}
			var missing []string
			for k := range set {
				missing = append(missing, k)
			}
			sort.Strings(missing)
			skipped = append(skipped, skip{name, missing})
			continue
		}
		_ = cliQuiet("claude", "mcp", "remove", name, "-s", "user")
		if e = cliQuiet("claude", "mcp", "add-json", name, strings.TrimSpace(string(b)), "-s", "user"); e != nil {
			skipped = append(skipped, skip{name, []string{"ошибка регистрации (вывод скрыт)"}})
		} else {
			registered = append(registered, name)
		}
	}
	if len(registered) > 0 {
		fmt.Println("  MCP зарегистрированы:", strings.Join(registered, ", "))
	}
	for _, s := range skipped {
		fmt.Printf("  MCP пропущен: %s (нет %s)\n", s.name, strings.Join(s.why, ", "))
	}
	return nil
}
func claudeSync(root string) error {
	dir := envDefault([]string{"CLAUDE_DIR"}, envDefault([]string{"CLAUDE_CONFIG_DIR"}, filepath.Join(home(), ".claude")))
	dst := envDefault([]string{"DST"}, filepath.Join(root, "config/claude"))
	settings, e := readObject(filepath.Join(dir, "settings.json"), false)
	if e != nil {
		return e
	}
	omc, e := readObject(filepath.Join(dir, ".omc-config.json"), false)
	if e != nil {
		return e
	}
	cj, e := readObject(filepath.Join(home(), ".claude.json"), false)
	if e != nil {
		return e
	}
	permissions, ok := settings["permissions"].(map[string]any)
	if !ok {
		return errors.New("invalid permissions object")
	}
	allow := []any{}
	if v, ok := permissions["allow"]; ok {
		var yes bool
		allow, yes = v.([]any)
		if !yes {
			return errors.New("invalid permissions allow list")
		}
	}
	kept := []any{}
	for _, v := range allow {
		s, ok := v.(string)
		if !ok {
			return errors.New("invalid permission")
		}
		if !strings.Contains(s, "/Users/") {
			kept = append(kept, s)
		}
	}
	permissions["allow"] = kept
	if v, ok := settings["env"]; ok && v != nil {
		m, ok := v.(map[string]any)
		if !ok {
			return errors.New("invalid settings env")
		}
		if _, ok = m["ANTHROPIC_BASE_URL"]; ok {
			m["ANTHROPIC_BASE_URL"] = "${ANTHROPIC_BASE_URL}"
		}
	}
	servers := map[string]any{}
	if v, ok := cj["mcpServers"]; ok {
		var yes bool
		servers, yes = v.(map[string]any)
		if !yes {
			return errors.New("invalid mcpServers object")
		}
	}
	var warnings, names []string
	for name, v := range servers {
		cfg, ok := v.(map[string]any)
		if !ok {
			return errors.New("invalid MCP configuration")
		}
		names = append(names, name)
		if v, ok := cfg["env"]; ok && v != nil {
			env, ok := v.(map[string]any)
			if !ok {
				return errors.New("invalid MCP environment")
			}
			for k := range env {
				secret := ""
				if name == "zai-mcp-server" && k == "Z_AI_API_KEY" {
					secret = "Z_AI_API_KEY"
				} else if name == "magic" && k == "API_KEY" {
					secret = "MAGIC_API_KEY"
				} else if secretKey.MatchString(k) {
					secret = k
					warnings = append(warnings, fmt.Sprintf("%s.env.%s: замаскирован как ${%s} — добавьте его в ~/.claude/.mcp-secrets и в SECRET_MAP скриптов", name, k, k))
				}
				if secret != "" {
					env[k] = "${" + secret + "}"
				}
			}
		}
		if v, ok := cfg["args"]; ok {
			args, ok := v.([]any)
			if !ok {
				return errors.New("invalid MCP args")
			}
			for i, a := range args {
				if s, ok := a.(string); ok && strings.Contains(s, "/Users/") && (strings.HasSuffix(s, ".yaml") || strings.HasSuffix(s, ".yml")) {
					args[i] = "${KUBECONFIG_PATH}"
				}
			}
		}
	}
	// Validate before writing: the shell predecessor detected settings leaks only
	// after exporting them. Refuse the whole export without publishing secrets.
	for _, d := range append([]map[string]any{settings}, serverObjects(servers)...) {
		if env, ok := d["env"].(map[string]any); ok {
			for k, v := range env {
				if s, ok := v.(string); ok && secretKey.MatchString(k) && !strings.HasPrefix(s, "${") {
					return errors.New("unmasked secret environment value; export unchanged")
				}
			}
		}
	}
	sb := encode(settings)
	if home() != "" {
		sb = []byte(strings.ReplaceAll(string(sb), home(), "$HOME"))
	}
	var changes []change
	for _, v := range []struct {
		name string
		b    []byte
	}{{"settings.json", sb}, {"omc-config.json", encode(omc)}, {"mcp-servers.json", encode(servers)}} {
		p := filepath.Join(dst, v.name)
		changes = append(changes, change{path: p, data: v.b, follow: true})
	}
	if e = commit(changes); e != nil {
		return e
	}
	sort.Strings(names)
	sort.Strings(warnings)
	fmt.Printf("  settings.json: allow %d -> %d, base_url -> плейсхолдер\n", len(allow), len(kept))
	fmt.Println("  omc-config.json: записан")
	fmt.Println("  mcp-servers.json: серверы ->", strings.Join(names, ", "))
	for _, w := range warnings {
		fmt.Println("  ⚠", w)
	}
	fmt.Println("  ✓ секреты замаскированы")
	return nil
}
func serverObjects(m map[string]any) []map[string]any {
	var out []map[string]any
	for _, v := range m {
		if d, ok := v.(map[string]any); ok {
			out = append(out, d)
		}
	}
	return out
}
