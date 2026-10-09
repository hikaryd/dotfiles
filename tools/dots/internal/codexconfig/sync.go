package codexconfig

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var envNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func exactSecret(p []string) string {
	switch pathID(p) {
	case "mcp_servers\x00zai-mcp-server\x00env\x00Z_AI_API_KEY":
		return "Z_AI_API_KEY"
	case "model_providers\x00cf-openai\x00base_url":
		return "CF_OPENAI_BASE_URL"
	}
	return ""
}
func safeEnv(p []string) bool {
	return pathID(p) == "mcp_servers\x00zai-mcp-server\x00env\x00Z_AI_MODE"
}
func envReference(p []string) bool {
	return len(p) > 0 && (p[len(p)-1] == "bearer_token_env_var" || p[len(p)-1] == "env_key" || contains(p, "env_http_headers"))
}
func sanitize(v any, p []string, home string, placeholders map[string]bool) (any, error) {
	last := p[len(p)-1]
	_, list := v.([]any)
	ref := last == "bearer_token_env_var" || last == "env_key" || (contains(p, "env_http_headers") && last != "env_http_headers") || (last == "env_vars" && !list)
	s, isString := v.(string)
	dotted := strings.Join(p, ".")
	if ref {
		if !isString || !envNameRE.MatchString(s) {
			return nil, fmt.Errorf("environment reference must be a shell variable name: %s", dotted)
		}
		return s, nil
	}
	if secret := exactSecret(p); secret != "" {
		if !isString {
			return nil, fmt.Errorf("configured secret value must be a string: %s", dotted)
		}
		placeholders[secret] = true
		return "${" + secret + "}", nil
	}
	if len(p) == 3 && p[0] == "shell_environment_policy" && p[1] == "set" {
		if !envNameRE.MatchString(last) {
			return nil, fmt.Errorf("shell environment key is not a valid variable: %s", last)
		}
		if !isString {
			return nil, fmt.Errorf("shell environment value must be a string: %s", last)
		}
		placeholders[last] = true
		return "${" + last + "}", nil
	}
	if len(p) == 4 && p[0] == "mcp_servers" && p[2] == "env" {
		if !envNameRE.MatchString(last) {
			return nil, fmt.Errorf("MCP environment key is not a valid shell variable: %s", last)
		}
		if !isString {
			return nil, fmt.Errorf("MCP environment value must be a string: %s", last)
		}
		if safeEnv(p) {
			return s, nil
		}
		placeholders[last] = true
		return "${" + last + "}", nil
	}
	switch x := v.(type) {
	case object:
		out := object{}
		for _, c := range x {
			value, e := sanitize(c.value, appendPath(p, c.key), home, placeholders)
			if e != nil {
				return nil, e
			}
			out = append(out, entry{c.key, value})
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, c := range x {
			value, e := sanitize(c, p, home, placeholders)
			if e != nil {
				return nil, e
			}
			out[i] = value
		}
		return out, nil
	case string:
		if home != "" {
			return strings.ReplaceAll(x, home, "${HOME}"), nil
		}
	}
	return v, nil
}
func syncConfig(source, target, home string) (changed bool, names []string, err error) {
	if err = distinct(source, target, "source and snapshot target"); err != nil {
		return
	}
	live, err := readText(source)
	if err != nil {
		return false, nil, err
	}
	parsed, err := orderedDecode(live, source)
	if err != nil {
		return false, nil, err
	}
	var rootLines []string
	for _, k := range rootKeys {
		if v, ok := parsed.get(k); ok {
			s, e := tomlValue(v)
			if e != nil {
				return false, nil, e
			}
			rootLines = append(rootLines, k+" = "+s)
		}
	}
	var tables []tableBlock
	ph := map[string]bool{}
	for _, prefix := range managedPrefixes {
		v, _ := parsed.get(prefix)
		o, ok := v.(object)
		if !ok {
			continue
		}
		sanitized, e := sanitize(o, []string{prefix}, home, ph)
		if e != nil {
			return false, nil, e
		}
		blocks, e := serializeTree([]string{prefix}, sanitized.(object))
		if e != nil {
			return false, nil, e
		}
		tables = append(tables, blocks...)
	}
	if v, ok := parsed.get("tui"); ok {
		if o, ok := v.(object); ok {
			if status, ok := o.get("status_line"); ok {
				s, e := tomlValue(status)
				if e != nil {
					return false, nil, e
				}
				tables = append(tables, tableBlock{[]string{"tui"}, []string{"[tui]", "status_line = " + s}})
			}
		}
	}
	header := "# Stable personal Codex settings managed by ~/dots.\n# Runtime/project/plugin state is intentionally preserved only in ~/.codex/config.toml."
	parts := []string{header + "\n" + strings.Join(rootLines, "\n")}
	for _, b := range tables {
		parts = append(parts, b.text())
	}
	snapshot := joinParts(parts...)
	for _, n := range placeholders(snapshot) {
		ph[n] = true
	}
	if err = leakGate(snapshot, home); err != nil {
		return
	}
	previous, exists, err := readExisting(target)
	if err != nil {
		return false, nil, err
	}
	changed = !exists || previous != snapshot
	if err = unchanged(source, true, live); err != nil {
		return
	}
	if err = unchanged(target, exists, previous); err != nil {
		return
	}
	if changed {
		mode := os.FileMode(0644)
		if exists {
			var info os.FileInfo
			info, err = os.Stat(target)
			if err != nil {
				return
			}
			mode = info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
		}
		err = atomicWrite(target, snapshot, mode)
	}
	return changed, setKeys(ph), err
}
