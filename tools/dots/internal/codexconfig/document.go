package codexconfig

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
)

var rootKeys = []string{"approval_policy", "sandbox_mode", "model", "model_provider", "personality", "suppress_unstable_features_warning", "service_tier", "model_reasoning_effort"}
var managedPrefixes = []string{"shell_environment_policy", "mcp_servers", "agents", "model_providers"}

const (
	rootStart    = "# >>> dots Codex managed root >>>"
	rootEnd      = "# <<< dots Codex managed root <<<"
	tableStart   = "# >>> dots Codex managed table >>>"
	tableEnd     = "# <<< dots Codex managed table <<<"
	partialStart = "# >>> dots Codex managed keys >>>"
	partialEnd   = "# <<< dots Codex managed keys <<<"
	probeKey     = "__dots_codex_table_probe_7fdd74d1__"
)

type tableBlock struct {
	path  []string
	lines []string
}

func (b tableBlock) text() string { return strings.Join(b.lines, "\n") }

type document struct {
	root   []string
	tables []tableBlock
}
type lexState struct {
	delimiter     string
	square, curly int
}

func (s *lexState) level() bool { return s.delimiter == "" && s.square == 0 && s.curly == 0 }
func escaped(s string, i int) bool {
	n := 0
	for i--; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}
func (s *lexState) scan(line string) {
	for i := 0; i < len(line); {
		if s.delimiter != "" {
			end := strings.Index(line[i:], s.delimiter)
			if end < 0 {
				return
			}
			end += i
			for s.delimiter == `"""` && escaped(line, end) {
				n := strings.Index(line[end+1:], s.delimiter)
				if n < 0 {
					return
				}
				end += n + 1
			}
			i = end + len(s.delimiter)
			s.delimiter = ""
			continue
		}
		if strings.HasPrefix(line[i:], `"""`) || strings.HasPrefix(line[i:], "'''") {
			s.delimiter = line[i : i+3]
			i += 3
			continue
		}
		switch line[i] {
		case '#':
			return
		case '"':
			i++
			for i < len(line) {
				if line[i] == '"' && !escaped(line, i) {
					i++
					break
				}
				i++
			}
			continue
		case '\'':
			n := strings.IndexByte(line[i+1:], '\'')
			if n < 0 {
				return
			}
			i += n + 2
			continue
		case '[':
			s.square++
		case ']':
			s.square = max(0, s.square-1)
		case '{':
			s.curly++
		case '}':
			s.curly = max(0, s.curly-1)
		}
		i++
	}
}
func decode(text string) (map[string]any, error) {
	m := map[string]any{}
	if !utf8.ValidString(text) {
		return nil, fmt.Errorf("invalid UTF-8")
	}
	err := toml.Unmarshal([]byte(text), &m)
	return m, err
}
func validate(text, label string) (map[string]any, error) {
	m, e := decode(text)
	if e != nil {
		// Parser diagnostics can include values from a secret-bearing configuration.
		if d, ok := e.(*toml.DecodeError); ok {
			row, col := d.Position()
			return nil, fmt.Errorf("%s: invalid TOML (line %d, column %d)", label, row, col)
		}
		return nil, fmt.Errorf("%s: invalid TOML", label)
	}
	return m, nil
}
func findProbe(v any, path []string) []string {
	switch x := v.(type) {
	case map[string]any:
		if _, ok := x[probeKey]; ok {
			return path
		}
		for k, c := range x {
			if p := findProbe(c, appendPath(path, k)); p != nil {
				return p
			}
		}
	case []any:
		for _, c := range x {
			if p := findProbe(c, path); p != nil {
				return p
			}
		}
	}
	return nil
}
func tablePath(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[") {
		return nil
	}
	m, e := decode(line + "\n" + probeKey + " = true\n")
	if e != nil {
		return nil
	}
	return findProbe(m, []string{})
}
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}
func parseDocument(text string) document {
	d := document{}
	s := lexState{}
	for _, line := range splitLines(text) {
		var p []string
		if s.level() {
			p = tablePath(line)
		}
		if p != nil {
			d.tables = append(d.tables, tableBlock{p, []string{line}})
		} else if len(d.tables) == 0 {
			d.root = append(d.root, line)
		} else {
			i := len(d.tables) - 1
			d.tables[i].lines = append(d.tables[i].lines, line)
		}
		s.scan(line)
	}
	return d
}
func markerPair(lines []string, start, end string) bool {
	s := lexState{}
	seen := false
	for _, l := range lines {
		if s.level() {
			if strings.TrimSpace(l) == start {
				seen = true
			} else if strings.TrimSpace(l) == end && seen {
				return true
			}
		}
		s.scan(l)
	}
	return false
}
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func appendPath(p []string, s string) []string { return append(append([]string{}, p...), s) }
func pathID(p []string) string                 { return strings.Join(p, "\x00") }
func managed(p []string) bool                  { return len(p) > 0 && contains(managedPrefixes, p[0]) }
func partial(p []string) bool                  { return len(p) == 1 && p[0] == "tui" }
func group(p []string) []string {
	if len(p) >= 2 && (p[0] == "mcp_servers" || p[0] == "model_providers") {
		return p[:2]
	}
	return p[:min(1, len(p))]
}
func nodeAt(m map[string]any, p []string) any {
	var v any = m
	for _, k := range p {
		x, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = x[k]
	}
	return v
}
func validateScope(d document, parsed map[string]any) error {
	r, e := validate(strings.Join(d.root, "\n"), "Codex template root")
	if e != nil {
		return e
	}
	var roots, tables, keys []string
	for k := range r {
		if !contains(rootKeys, k) {
			roots = append(roots, k)
		}
	}
	for _, b := range d.tables {
		if !managed(b.path) && !partial(b.path) {
			tables = append(tables, strings.Join(b.path, "."))
		}
	}
	if m, ok := parsed["tui"].(map[string]any); ok {
		for k := range m {
			if k != "status_line" {
				keys = append(keys, "tui."+k)
			}
		}
	}
	var details []string
	for _, item := range []struct {
		label  string
		values []string
	}{{"root keys", roots}, {"tables", tables}, {"partial keys", keys}} {
		if len(item.values) > 0 {
			details = append(details, item.label+": "+strings.Join(sorted(item.values), ", "))
		}
	}
	if len(details) > 0 {
		return fmt.Errorf("template contains unmanaged settings (%s)", strings.Join(details, "; "))
	}
	return nil
}
func joinParts(parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.Trim(p, "\n"))
		}
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n\n") + "\n"
}
func meaningful(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
func cleanRoot(lines []string, unmarked bool) ([]string, error) {
	var kept []string
	inside := false
	s := lexState{}
	for i := 0; i < len(lines); {
		l := lines[i]
		level := s.level()
		if level && strings.TrimSpace(l) == rootStart {
			if inside {
				return nil, fmt.Errorf("nested managed root marker: %s", rootStart)
			}
			inside = true
			s.scan(l)
			i++
			continue
		}
		if level && strings.TrimSpace(l) == rootEnd && inside {
			inside = false
			s.scan(l)
			i++
			continue
		}
		if inside {
			s.scan(l)
			i++
			continue
		}
		removed := false
		if unmarked && level {
			for end := i; end < len(lines); end++ {
				m, e := decode(strings.Join(lines[i:end+1], "\n") + "\n")
				if e != nil {
					continue
				}
				for k := range m {
					if contains(rootKeys, k) {
						i = end + 1
						removed = true
						break
					}
				}
				break
			}
		}
		if removed {
			continue
		}
		kept = append(kept, l)
		s.scan(l)
		i++
	}
	if inside {
		return nil, fmt.Errorf("unterminated managed root marker: %s", rootStart)
	}
	return kept, nil
}
func cleanPartial(b tableBlock) tableBlock {
	kept := []string{b.lines[0]}
	s := lexState{}
	s.scan(b.lines[0])
	for i := 1; i < len(b.lines); {
		l := b.lines[i]
		level := s.level()
		if level && (strings.TrimSpace(l) == partialStart || strings.TrimSpace(l) == partialEnd) {
			s.scan(l)
			i++
			continue
		}
		endRemove := -1
		if level {
			for end := i; end < len(b.lines); end++ {
				m, e := decode(b.lines[0] + "\n" + strings.Join(b.lines[i:end+1], "\n") + "\n")
				if e != nil {
					continue
				}
				if n, ok := nodeAt(m, b.path).(map[string]any); ok {
					if _, ok := n["status_line"]; ok {
						endRemove = end
					}
				}
				break
			}
		}
		if endRemove >= 0 {
			i = endRemove + 1
			continue
		}
		kept = append(kept, l)
		s.scan(l)
		i++
	}
	return tableBlock{b.path, kept}
}
func mark(b tableBlock, start, end string, extra []string) tableBlock {
	lines := []string{b.lines[0], start}
	lines = append(lines, b.lines[1:]...)
	lines = append(lines, end)
	lines = append(lines, extra...)
	return tableBlock{b.path, lines}
}
