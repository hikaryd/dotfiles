package setup

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func shellQuote(s string) string {
	if s != "" && regexp.MustCompile(`^[a-zA-Z0-9_@%+=:,./-]+$`).MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
func taskCommand() (string, error) {
	binary, e := os.Executable()
	if e != nil {
		return "", e
	}
	return shellQuote(binary) + " agents codex-task-title hook # dots-task-title", nil
}
func isManagedTask(v any) bool {
	s, ok := v.(string)
	return ok && strings.HasSuffix(strings.TrimRight(s, " \t\r\n"), "# dots-task-title")
}
func hookGroups(d map[string]any, event string) (map[string]any, []any, error) {
	var hooks map[string]any
	if v, ok := d["hooks"]; ok {
		hooks, ok = v.(map[string]any)
		if !ok {
			return nil, nil, errors.New("invalid hooks object")
		}
	} else {
		hooks = map[string]any{}
		d["hooks"] = hooks
	}
	groups := []any{}
	if v, ok := hooks[event]; ok {
		var yes bool
		groups, yes = v.([]any)
		if !yes {
			return nil, nil, errors.New("invalid hook groups")
		}
	}
	return hooks, groups, nil
}
func groupEntries(v any) (map[string]any, []any, error) {
	group, ok := v.(map[string]any)
	if !ok {
		return nil, nil, errors.New("invalid hook group")
	}
	entries, ok := group["hooks"].([]any)
	if !ok {
		return nil, nil, errors.New("invalid hook entries")
	}
	for _, v := range entries {
		if _, ok := v.(map[string]any); !ok {
			return nil, nil, errors.New("invalid hook entry")
		}
	}
	return group, entries, nil
}
func mergeTaskHooks(original []byte, command string) ([]byte, error) {
	d := map[string]any{}
	var e error
	if len(original) > 0 {
		d, e = object(original)
		if e != nil {
			return nil, e
		}
	}
	hooks, groups, e := hookGroups(d, "UserPromptSubmit")
	if e != nil {
		return nil, e
	}
	expected := map[string]any{"type": "command", "command": command, "timeout": 5}
	count := 0
	for _, v := range groups {
		_, entries, e := groupEntries(v)
		if e != nil {
			return nil, e
		}
		for i, v := range entries {
			entry := v.(map[string]any)
			if isManagedTask(entry["command"]) {
				count++
				entries[i] = expected
			}
		}
	}
	if count > 1 {
		return nil, errors.New("Multiple managed task-title hooks found; configuration unchanged")
	}
	if count == 0 {
		groups = append(groups, map[string]any{"hooks": []any{expected}})
	}
	hooks["UserPromptSubmit"] = groups
	return encode(d), nil
}
func abideCommands(root string) []string {
	var out []string
	for _, n := range []string{"session-start", "turn-start", "post-tool-use", "stop"} {
		out = append(out, shellQuote(absolute(filepath.Join(root, "scripts/abide")))+" --hook "+n+" # abide-hook.js")
	}
	return out
}
func mergeAbideHooks(original []byte, root string) ([]byte, error) {
	d := map[string]any{}
	var e error
	if original != nil {
		d, e = object(original)
		if e != nil {
			return nil, e
		}
	}
	commands := abideCommands(root)
	for i, event := range []string{"SessionStart", "UserPromptSubmit", "PostToolUse", "Stop"} {
		hooks, groups, e := hookGroups(d, event)
		if e != nil {
			return nil, e
		}
		kept := []any{}
		for _, v := range groups {
			group, entries, e := groupEntries(v)
			if e != nil {
				return nil, e
			}
			newEntries := []any{}
			for _, v := range entries {
				entry := v.(map[string]any)
				s, _ := entry["command"].(string)
				if !strings.Contains(s, "abide-hook.js") {
					newEntries = append(newEntries, entry)
				}
			}
			if len(newEntries) > 0 {
				group["hooks"] = newEntries
				kept = append(kept, group)
			}
		}
		timeout := []int{10, 10, 20, 30}[i]
		group := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": commands[i], "timeout": timeout}}}
		if event == "PostToolUse" {
			group["matcher"] = "Edit|Write|MultiEdit|apply_patch"
		}
		hooks[event] = append(kept, group)
	}
	return encode(d), nil
}
func parseTOML(s string) (map[string]any, error) {
	var d map[string]any
	if e := toml.Unmarshal([]byte(s), &d); e != nil {
		return nil, errors.New("invalid Codex TOML configuration")
	}
	if d == nil {
		d = map[string]any{}
	}
	return d, nil
}
func nested(m map[string]any, keys ...string) map[string]any {
	for _, k := range keys {
		m, _ = m[k].(map[string]any)
		if m == nil {
			return map[string]any{}
		}
	}
	return m
}

type tableValue struct {
	key   string
	value any
}

func replaceTable(text, name string, values []tableValue) string {
	header := "[" + name + "]"
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(header) + `[^\n]*\n`)
	loc := re.FindStringIndex(text)
	body := ""
	start, end := len(text), len(text)
	if loc != nil {
		start = loc[0]
		end = len(text)
		if next := regexp.MustCompile(`(?m)^\[`).FindStringIndex(text[loc[1]:]); next != nil {
			end = loc[1] + next[0]
		}
		body = text[loc[1]:end]
	}
	for _, v := range values {
		b, _ := json.Marshal(v.value)
		line := v.key + " = " + string(b)
		re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(v.key) + `\s*=.*$`)
		if re.MatchString(body) {
			body = re.ReplaceAllStringFunc(body, func(string) string { return line })
		} else {
			body = strings.TrimRight(body, " \t\r\n") + "\n" + line + "\n"
		}
	}
	block := header + "\n" + strings.TrimRight(strings.TrimLeft(body, "\n"), " \t\r\n") + "\n\n"
	if loc != nil {
		return text[:start] + block + text[end:]
	}
	return strings.TrimRight(text, " \t\r\n") + "\n\n" + block
}

type hookEntry struct {
	Key         string `json:"key"`
	SourcePath  string `json:"sourcePath"`
	Command     string `json:"command"`
	CurrentHash string `json:"currentHash"`
	TrustStatus string `json:"trustStatus"`
	Enabled     bool   `json:"enabled"`
}

func listHooks(root, binary, directory string) ([]hookEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "app-server")
	cmd.Env = append(os.Environ(), "CODEX_HOME="+directory)
	cmd.WaitDelay = time.Second
	stdin, e := cmd.StdinPipe()
	if e != nil {
		return nil, errors.New("cannot start Codex hook discovery")
	}
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		return nil, errors.New("cannot start Codex hook discovery")
	}
	if e = cmd.Start(); e != nil {
		return nil, errors.New("cannot start Codex hook discovery")
	}
	defer func() { stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	type lineResult struct {
		data []byte
		err  error
	}
	lines := make(chan lineResult, 1)
	go func() {
		scan := bufio.NewScanner(stdout)
		scan.Buffer(make([]byte, 65536), 8<<20)
		for scan.Scan() {
			b := append([]byte(nil), scan.Bytes()...)
			select {
			case lines <- lineResult{data: b}:
			case <-ctx.Done():
				return
			}
		}
		select {
		case lines <- lineResult{err: errors.New("Codex hook discovery stream ended")}:
		case <-ctx.Done():
		}
	}()
	send := func(v any) error {
		b, e := json.Marshal(v)
		if e != nil {
			return e
		}
		_, e = stdin.Write(append(b, '\n'))
		if e != nil {
			return errors.New("Codex hook discovery request failed")
		}
		return nil
	}
	response := func(id int) (json.RawMessage, error) {
		timer := time.NewTimer(20 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil, errors.New("Codex hook discovery timed out")
			case <-timer.C:
				return nil, errors.New("Codex hook discovery timed out")
			case line := <-lines:
				if line.err != nil {
					return nil, line.err
				}
				var payload struct {
					ID     int             `json:"id"`
					Result json.RawMessage `json:"result"`
					Error  json.RawMessage `json:"error"`
				}
				if json.Unmarshal(line.data, &payload) != nil {
					return nil, errors.New("invalid Codex hook response")
				}
				if payload.ID == id {
					if payload.Error != nil {
						return nil, errors.New("Codex rejected hooks/list")
					}
					return payload.Result, nil
				}
			}
		}
	}
	if e = send(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]any{"name": "dots_abide", "version": "1"}, "capabilities": map[string]any{"experimentalApi": true}}}); e != nil {
		return nil, e
	}
	if _, e = response(1); e != nil {
		return nil, e
	}
	if e = send(map[string]any{"method": "initialized", "params": map[string]any{}}); e != nil {
		return nil, e
	}
	if e = send(map[string]any{"id": 2, "method": "hooks/list", "params": map[string]any{"cwds": []string{absolute(root)}}}); e != nil {
		return nil, e
	}
	raw, e := response(2)
	if e != nil {
		return nil, e
	}
	var result struct {
		Data []struct {
			Errors []json.RawMessage `json:"errors"`
			Hooks  []hookEntry       `json:"hooks"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Data) != 1 || len(result.Data[0].Errors) != 0 {
		return nil, errors.New("Codex reported hook configuration errors")
	}
	seenKeys := map[string]bool{}
	for _, h := range result.Data[0].Hooks {
		if h.Key == "" || h.SourcePath == "" || h.CurrentHash == "" {
			return nil, errors.New("incomplete Codex hook response")
		}
		if seenKeys[h.Key] {
			return nil, errors.New("duplicate Codex hook identity")
		}
		seenKeys[h.Key] = true
	}
	return result.Data[0].Hooks, nil
}
func sameSource(p, q string) bool {
	a, e := resolve(p)
	if e != nil {
		return false
	}
	b, e := resolve(q)
	return e == nil && a == b
}
func otherStates(entries []hookEntry, ours func(hookEntry) bool) map[string]any {
	m := map[string]any{}
	for _, h := range entries {
		if !ours(h) {
			m[h.Key] = []any{h.CurrentHash, h.TrustStatus, h.Enabled}
		}
	}
	return m
}
func regularOptional(p string) ([]byte, error) {
	i, e := os.Lstat(p)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if !i.Mode().IsRegular() {
		return nil, errors.New("expected a regular configuration file")
	}
	return os.ReadFile(p)
}
func writeHookFile(p string, b []byte, group string, mode os.FileMode) error {
	return commit([]change{{path: p, data: b, backup: privateBackup(group, p), backupPrivate: true, mode: mode}})
}
func enableHooks(s string) (string, error) {
	d, e := parseTOML(s)
	if e != nil {
		return "", e
	}
	if nested(d, "features")["hooks"] != true {
		if _, ok := d["features"]; ok && !strings.Contains(s, "[features]") {
			return "", errors.New("Nonstandard features table; configuration unchanged")
		}
		s = replaceTable(s, "features", []tableValue{{"hooks", true}})
	}
	_, e = parseTOML(s)
	return s, e
}
func trustUpdate(current string, ours []hookEntry, requireEnabled bool) (string, error) {
	d, e := parseTOML(current)
	if e != nil {
		return "", e
	}
	state := nested(d, "hooks", "state")
	updated := current
	for _, h := range ours {
		existing, _ := state[h.Key].(map[string]any)
		enabledOK := existing["enabled"] != false
		if requireEnabled {
			enabledOK = existing["enabled"] == true
		}
		if existing["trusted_hash"] != h.CurrentHash || !enabledOK {
			key, _ := json.Marshal(h.Key)
			name := "hooks.state." + string(key)
			if len(existing) > 0 && !strings.Contains(current, "["+name+"]") {
				return "", errors.New("Nonstandard hook trust table; configuration unchanged")
			}
			updated = replaceTable(updated, name, []tableValue{{"trusted_hash", h.CurrentHash}, {"enabled", true}})
		}
	}
	after, e := parseTOML(updated)
	if e != nil {
		return "", e
	}
	beforeOther := map[string]any{}
	afterOther := map[string]any{}
	keys := map[string]bool{}
	for _, h := range ours {
		keys[h.Key] = true
	}
	for k, v := range state {
		if !keys[k] {
			beforeOther[k] = v
		}
	}
	for k, v := range nested(after, "hooks", "state") {
		if !keys[k] {
			afterOther[k] = v
		}
	}
	if !reflect.DeepEqual(beforeOther, afterOther) {
		return "", errors.New("refusing to change another hook's trust configuration")
	}
	return updated, nil
}
func unchanged(p string, expected []byte) error {
	b, e := readOptional(p)
	if e != nil {
		return e
	}
	if string(b) != string(expected) {
		return errors.New("configuration changed during installation")
	}
	return nil
}
func taskTitleApply(root string) error {
	binary, e := exec.LookPath("codex")
	if e != nil {
		return errors.New("Codex CLI is required to validate hook trust before installation")
	}
	dir, e := resolve(codexDir())
	if e != nil {
		return e
	}
	hp, cp := filepath.Join(dir, "hooks.json"), filepath.Join(dir, "config.toml")
	original, e := regularOptional(hp)
	if e != nil {
		return e
	}
	config, e := regularOptional(cp)
	if e != nil {
		return e
	}
	if original != nil && len(original) == 0 {
		return errors.New("empty hook configuration")
	}
	command, e := taskCommand()
	if e != nil {
		return e
	}
	updated, e := mergeTaskHooks(original, command)
	if e != nil {
		return e
	}
	enabled, e := enableHooks(string(config))
	if e != nil {
		return e
	}
	if e = unchanged(hp, original); e != nil {
		return e
	}
	if e = unchanged(cp, config); e != nil {
		return e
	}
	if e = writeHookFile(cp, []byte(enabled), "task-titles", 0); e != nil {
		return e
	}
	baseline, e := listHooks(root, binary, dir)
	if e != nil {
		return e
	}
	if e = unchanged(hp, original); e != nil {
		return e
	}
	if e = writeHookFile(hp, updated, "task-titles", 0); e != nil {
		return e
	}
	entries, e := listHooks(root, binary, dir)
	if e != nil {
		return e
	}
	owned := func(h hookEntry) bool { return sameSource(h.SourcePath, hp) && isManagedTask(h.Command) }
	var ours []hookEntry
	for _, h := range entries {
		if sameSource(h.SourcePath, hp) && h.Command == command {
			ours = append(ours, h)
		}
	}
	if len(ours) != 1 {
		return errors.New("Codex did not discover the single expected task-title hook")
	}
	if !reflect.DeepEqual(otherStates(baseline, owned), otherStates(entries, owned)) {
		return errors.New("Other hook states changed; task-title trust was not granted")
	}
	current, e := regularOptional(cp)
	if e != nil {
		return e
	}
	trusted, e := trustUpdate(string(current), ours, true)
	if e != nil {
		return e
	}
	if e = unchanged(cp, current); e != nil {
		return e
	}
	if e = writeHookFile(cp, []byte(trusted), "task-titles", 0); e != nil {
		return e
	}
	verified, e := listHooks(root, binary, dir)
	if e != nil {
		return e
	}
	approved := []hookEntry{}
	for _, h := range verified {
		if h.Key == ours[0].Key && h.Command == command && sameSource(h.SourcePath, hp) {
			approved = append(approved, h)
		}
	}
	if len(approved) != 1 || approved[0].TrustStatus != "trusted" || !approved[0].Enabled || approved[0].CurrentHash != ours[0].CurrentHash {
		return errors.New("Codex did not confirm enabled/trusted task-title hook")
	}
	if !reflect.DeepEqual(otherStates(baseline, owned), otherStates(verified, owned)) {
		return errors.New("Other hook states changed; inspect the configuration backup")
	}
	fmt.Printf("Task titles: enabled/trusted UserPromptSubmit hook installed in %s; other hook states preserved\n", hp)
	return nil
}
func abideTrust(root, dir string) error {
	binary, e := exec.LookPath("codex")
	if e != nil {
		fmt.Println("Abide: Codex CLI не установлен; доверие к хукам настройте после установки через /hooks")
		return nil
	}
	cp := filepath.Join(dir, "config.toml")
	original, e := regularOptional(cp)
	if e != nil {
		return e
	}
	enabled, e := enableHooks(string(original))
	if e != nil {
		return e
	}
	if e = unchanged(cp, original); e != nil {
		return e
	}
	if e = writeHookFile(cp, []byte(enabled), "abide", 0600); e != nil {
		return e
	}
	entries, e := listHooks(root, binary, dir)
	if e != nil {
		return e
	}
	expected := map[string]bool{}
	for _, s := range abideCommands(root) {
		expected[s] = true
	}
	source := filepath.Join(dir, "hooks.json")
	owned := func(h hookEntry) bool { return sameSource(h.SourcePath, source) && expected[h.Command] }
	var ours []hookEntry
	seen := map[string]bool{}
	for _, h := range entries {
		if owned(h) {
			ours = append(ours, h)
			seen[h.Command] = true
		}
	}
	if len(ours) != 4 || len(seen) != 4 {
		return errors.New("Codex не обнаружил четыре ожидаемых глобальных хука Abide")
	}
	current, e := regularOptional(cp)
	if e != nil {
		return e
	}
	updated, e := trustUpdate(string(current), ours, false)
	if e != nil {
		return e
	}
	if e = unchanged(cp, current); e != nil {
		return e
	}
	if e = writeHookFile(cp, []byte(updated), "abide", 0600); e != nil {
		return e
	}
	verified, e := listHooks(root, binary, dir)
	if e != nil {
		return e
	}
	count := 0
	identities := map[string]hookEntry{}
	for _, h := range ours {
		identities[h.Key] = h
	}
	for _, h := range verified {
		if owned(h) {
			count++
			before := identities[h.Key]
			if h.TrustStatus != "trusted" || !h.Enabled || before.CurrentHash != h.CurrentHash || before.Command != h.Command {
				return errors.New("Codex не подтвердил доверие к четырём хукам Abide")
			}
		}
	}
	if count != 4 {
		return errors.New("Codex не подтвердил доверие к четырём хукам Abide")
	}
	if !reflect.DeepEqual(otherStates(entries, owned), otherStates(verified, owned)) {
		return errors.New("состояние сторонних хуков изменилось; нужна проверка")
	}
	fmt.Println("Abide: Codex подтвердил четыре enabled/trusted хука; стороннее доверие сохранено")
	return nil
}
