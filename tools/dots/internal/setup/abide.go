package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type runtimePatch struct{ path, before, after string }

func patchRuntime(pkg string) (int, error) {
	manifest, e := regularOptional(filepath.Join(pkg, "package.json"))
	if e != nil {
		return 0, e
	}
	m, e := object(manifest)
	if e != nil {
		return 0, errors.New("invalid Abide package manifest")
	}
	if m["name"] != "@coldtea/abide" || m["version"] != "0.0.7" {
		return 0, errors.New("Runtime fixes require @coldtea/abide 0.0.7")
	}
	originals := map[string]string{}
	updated := map[string]string{}
	var order []string
	for _, patch := range runtimePatches {
		p := filepath.Join(pkg, patch.path)
		if _, ok := originals[p]; !ok {
			b, e := regularOptional(p)
			if e != nil || b == nil {
				return 0, errors.New("refusing to patch missing or non-regular Abide runtime")
			}
			originals[p] = string(b)
			updated[p] = string(b)
			order = append(order, p)
		}
		text := updated[p]
		oldCount, newCount := strings.Count(text, patch.before), strings.Count(text, patch.after)
		if newCount == 1 && strings.Count(strings.Replace(text, patch.after, "", 1), patch.before) == 0 {
			continue
		}
		if oldCount == 1 && newCount == 0 {
			updated[p] = strings.Replace(text, patch.before, patch.after, 1)
			continue
		}
		return 0, fmt.Errorf("Unrecognized Abide runtime in %s; no patches applied", patch.path)
	}
	var changes []change
	for _, p := range order {
		if originals[p] != updated[p] {
			changes = append(changes, change{path: p, data: []byte(updated[p]), expect: true, expected: []byte(originals[p])})
		}
	}
	if e = commit(changes); e != nil {
		return 0, e
	}
	return len(changes), nil
}
func abideRuntime(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: abide-runtime-patch package")
	}
	n, e := patchRuntime(args[0])
	if e != nil {
		return e
	}
	fmt.Printf("Abide runtime fixes: %d files updated\n", n)
	return nil
}

var keyAssignment = regexp.MustCompile(`^(?:export\s+)?([A-Z_][A-Z0-9_]*)\s*=\s*(.*)$`)

func readKeys(p string) (map[string]string, error) {
	b, e := readOptional(p)
	if e != nil {
		return nil, errors.New("cannot read credential configuration")
	}
	keys := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		m := keyAssignment.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		v := strings.TrimSpace(m[2])
		if len(v) > 1 && v[0] == v[len(v)-1] && (v[0] == '\'' || v[0] == '"') {
			v = v[1 : len(v)-1]
		}
		if v != "" {
			keys[m[1]] = v
		}
	}
	return keys, nil
}
func configureCredentials() error {
	p := filepath.Join(home(), ".abide/.env")
	if i, e := os.Lstat(p); e == nil && i.Mode()&os.ModeSymlink != 0 {
		return errors.New("expected a regular credential file")
	}
	keys, e := readKeys(p)
	if e != nil {
		return e
	}
	available := map[string]string{}
	for _, name := range []string{"TYPESAFE_AI_API_KEY", "OPENROUTER_API_KEY", "AI_GATEWAY_API_KEY"} {
		available[name] = envDefault([]string{name}, keys[name])
	}
	direct, gateway := available["TYPESAFE_AI_API_KEY"], available["AI_GATEWAY_API_KEY"]
	if (direct != "" && !strings.HasPrefix(direct, "sk-or-")) || (direct == "" && gateway != "" && !strings.HasPrefix(gateway, "sk-or-") && available["OPENROUTER_API_KEY"] == "") {
		if _, e = os.Stat(p); e == nil {
			if e = os.Chmod(p, 0600); e != nil {
				return errors.New("cannot secure credential configuration")
			}
		}
		fmt.Println("Abide: существующие ключи сохранены")
		return nil
	}
	existing := map[string]string{}
	if direct == "" && gateway == "" && available["OPENROUTER_API_KEY"] == "" {
		existing, e = readKeys(filepath.Join(home(), ".config/jevgrep/secrets.env"))
		if e != nil {
			return e
		}
	}
	router := available["OPENROUTER_API_KEY"]
	if router == "" {
		for _, v := range []string{direct, gateway} {
			if strings.HasPrefix(v, "sk-or-") {
				router = v
				break
			}
		}
	}
	if router == "" {
		router = existing["OPENROUTER_API_KEY"]
	}
	if router == "" && strings.HasPrefix(existing["AI_GATEWAY_API_KEY"], "sk-or-") {
		router = existing["AI_GATEWAY_API_KEY"]
	}
	b, e := readOptional(p)
	if e != nil {
		return errors.New("cannot read credential configuration")
	}
	content := string(b)
	message := ""
	remove := func(name string) {
		re := regexp.MustCompile(`(?m)^(?:export\s+)?` + name + `\s*=.*\n?`)
		content = re.ReplaceAllString(content, "")
	}
	if router != "" {
		for _, name := range []string{"AI_GATEWAY_API_KEY", "TYPESAFE_AI_API_KEY"} {
			if keys[name] == router {
				remove(name)
			}
		}
		if keys["TYPESAFE_AI_BASE_URL"] == "https://openrouter.ai/api/v1" {
			remove("TYPESAFE_AI_BASE_URL")
		}
		re := regexp.MustCompile(`(?m)^(?:export\s+)?OPENROUTER_API_KEY\s*=.*$`)
		line := "OPENROUTER_API_KEY=" + router
		if re.MatchString(content) {
			content = re.ReplaceAllStringFunc(content, func(string) string { return line })
		} else {
			content = strings.TrimRight(content, " \t\r\n") + "\n" + line + "\n"
		}
		message = "Abide: OpenRouter → Jev System One API; ключ сохранён вне Git, значения не выводятся"
	} else if existing["AI_GATEWAY_API_KEY"] != "" {
		content = strings.TrimRight(content, " \t\r\n") + "\nAI_GATEWAY_API_KEY=" + existing["AI_GATEWAY_API_KEY"] + "\n"
		message = "Abide: сохранён существующий Gateway key; значения не выводятся"
	} else {
		fmt.Println("Abide: задайте OPENROUTER_API_KEY в ~/.abide/.env или выполните abide login с TypeSafe/Gateway key")
		return nil
	}
	if e = writeHookFile(p, []byte(content), "abide", 0600); e != nil {
		return errors.New("cannot save credential configuration")
	}
	if e = os.Chmod(filepath.Dir(p), 0700); e != nil {
		return errors.New("cannot secure credential directory")
	}
	fmt.Println(message)
	return nil
}
func abideApply(root string) error {
	codex := codexDir()
	claude := expand(envDefault([]string{"CLAUDE_CONFIG_DIR"}, filepath.Join(home(), ".claude")))
	changes, e := abideConfigurationChanges(root, codex, claude)
	if e != nil {
		return e
	}
	versionBytes, e := os.ReadFile(filepath.Join(root, "config/abide/version"))
	if e != nil {
		return e
	}
	version := strings.TrimSpace(string(versionBytes))
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][a-zA-Z0-9.-]+)?$`).MatchString(version) {
		return errors.New("invalid pinned Abide version")
	}
	prefix := filepath.Join(home(), ".local/share/dots/abide", version)
	pkg := filepath.Join(prefix, "node_modules/@coldtea/abide")
	manifest := filepath.Join(pkg, "package.json")
	metadata, e := readObject(manifest, true)
	if e != nil {
		return e
	}
	if metadata["version"] != version {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "npm", "install", "--prefix", prefix, "--ignore-scripts", "--no-audit", "--no-fund", "@coldtea/abide@"+version)
		cmd.WaitDelay = time.Second
		if e = cmd.Run(); e != nil {
			return errors.New("Abide package installation failed (output suppressed)")
		}
	}
	n, e := patchRuntime(pkg)
	if e != nil {
		return e
	}
	fmt.Printf("Abide: совместимость OpenRouter и диагностика проверены; обновлено файлов: %d\n", n)
	wrapper := absolute(filepath.Join(root, "scripts/abide"))
	link := filepath.Join(home(), ".local/bin/abide")
	if i, e := os.Lstat(link); e == nil {
		if i.Mode()&os.ModeSymlink == 0 || !sameSource(link, wrapper) {
			return errors.New("существующая команда не перезаписана")
		}
	} else if os.IsNotExist(e) {
		if e = commit([]change{{path: link, link: wrapper}}); e != nil {
			return e
		}
	} else {
		return e
	}
	if e = configureCredentials(); e != nil {
		return e
	}
	pluginBytes, e := os.ReadFile(filepath.Join(pkg, "opencode/abide.mjs"))
	if e != nil {
		return e
	}
	plugin := string(pluginBytes)
	needle := `spawn("node", [HOOK, name],`
	if strings.Count(plugin, needle) != 1 {
		return errors.New("структура плагина OpenCode изменилась; проверьте адаптацию proxy")
	}
	quoted := strings.TrimSpace(string(encode(wrapper)))
	plugin = strings.Replace(plugin, needle, "spawn("+quoted+`, ["--hook", name],`, 1)
	plugin = regexp.MustCompile(`(?m)^const HOOK = .*\n`).ReplaceAllString(plugin, "")
	plugin = "// abide-opencode-plugin: managed by dots-tools setup abide-apply\n" + plugin
	p := filepath.Join(home(), ".config/opencode/plugins/abide.js")
	changes = append(changes, change{path: p, data: []byte(plugin), backup: privateBackup("abide", p), backupPrivate: true})
	source, e := resolve(filepath.Join(pkg, "skills/abide-compile"))
	if e != nil {
		return e
	}
	sourceSkill, e := os.ReadFile(filepath.Join(source, "SKILL.md"))
	if e != nil {
		return e
	}
	for _, base := range []string{filepath.Join(home(), ".agents/skills"), filepath.Join(codex, "skills"), filepath.Join(claude, "skills"), filepath.Join(home(), ".config/opencode/skills")} {
		p := filepath.Join(base, "abide-compile")
		i, e := os.Lstat(p)
		if e == nil {
			if i.Mode()&os.ModeSymlink != 0 && sameSource(p, source) {
				continue
			}
			if i.IsDir() {
				b, e := os.ReadFile(filepath.Join(p, "SKILL.md"))
				if e == nil && string(b) == string(sourceSkill) {
					continue
				}
			}
			managed := false
			if i.Mode()&os.ModeSymlink != 0 {
				q, e := resolve(p)
				if e != nil {
					return e
				}
				base, e := resolve(filepath.Join(home(), ".local/share/dots/abide"))
				if e != nil {
					return e
				}
				rel, e := filepath.Rel(base, q)
				managed = e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && strings.HasSuffix(filepath.ToSlash(q), "/node_modules/@coldtea/abide/skills/abide-compile")
			}
			if !managed {
				return errors.New("существующий skill не перезаписан")
			}
		} else if !os.IsNotExist(e) {
			return e
		}
		changes = append(changes, change{path: p, link: source})
	}
	if e = commit(changes); e != nil {
		return e
	}
	for _, p := range []string{filepath.Join(codex, "hooks.json"), filepath.Join(claude, "settings.json")} {
		fmt.Printf("Abide: четыре хука установлены в %s\n", p)
	}
	if e = abideTrust(root, codex); e != nil {
		return e
	}
	fmt.Printf("Abide %s: глобальная установка готова\n", version)
	return nil
}

func abideConfigurationChanges(root, codex, claude string) ([]change, error) {
	var changes []change
	for _, p := range []string{filepath.Join(codex, "hooks.json"), filepath.Join(claude, "settings.json")} {
		b, e := regularOptional(p)
		if e != nil {
			return nil, e
		}
		updated, e := mergeAbideHooks(b, root)
		if e != nil {
			return nil, e
		}
		changes = append(changes, change{path: p, data: updated, backup: privateBackup("abide", p), backupPrivate: true, expect: true, expected: b})
	}
	settings, e := os.ReadFile(filepath.Join(root, "config/abide/instructions.md"))
	if e != nil {
		return nil, e
	}
	for _, p := range []string{filepath.Join(codex, "AGENTS.md"), filepath.Join(claude, "CLAUDE.md")} {
		b, e := regularOptional(p)
		if e != nil {
			return nil, e
		}
		s, e := instructionBlock(string(b), string(settings), "<!-- BEGIN DOTS ABIDE -->", "<!-- END DOTS ABIDE -->")
		if e != nil {
			return nil, e
		}
		changes = append(changes, change{path: p, data: []byte(s), backup: privateBackup("abide", p), backupPrivate: true, expect: true, expected: b})
	}
	// Invalid existing trust TOML must not trigger an install or credential edit.
	b, e := regularOptional(filepath.Join(codex, "config.toml"))
	if e != nil {
		return nil, e
	}
	if _, e = parseTOML(string(b)); e != nil {
		return nil, e
	}
	return changes, nil
}
