package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const titleSource = "dots:task-titles"

var ansi = regexp.MustCompile("\x1b(?:\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|\\[[0-?]*[ -/]*[@-~])")

func shorten(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	r = r[:limit-1]
	for i := len(r) - 1; i >= limit/2; i-- {
		if r[i] == ' ' {
			r = r[:i]
			break
		}
	}
	return strings.TrimRightFunc(string(r), unicode.IsSpace) + "…"
}
func cleanText(s string) string {
	s = ansi.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		// Python's category C also includes unassigned code points (Cn).
		// Go's unicode.C table omits those, so retain only assigned non-C classes.
		if !unicode.In(r, unicode.L, unicode.M, unicode.N, unicode.P, unicode.S, unicode.Z) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
func taskTitle(s string, limit int) string {
	s = cleanText(s)
	return shorten(s, limit)
}

type pane struct {
	ID            string        `json:"pane_id"`
	Terminal      string        `json:"terminal_id"`
	Tab           string        `json:"tab_id"`
	Agent         string        `json:"agent"`
	Native        string        `json:"terminal_title_stripped"`
	Cwd           string        `json:"cwd"`
	ForegroundCwd string        `json:"foreground_cwd"`
	Title         string        `json:"title"`
	Label         string        `json:"label"`
	Session       *agentSession `json:"agent_session"`
}
type agentSession struct {
	Agent string `json:"agent"`
	Value string `json:"value"`
}
type tab struct {
	ID    string `json:"tab_id"`
	Label string `json:"label"`
}
type layout struct {
	Tab   string `json:"tab_id"`
	Focus string `json:"focused_pane_id"`
}
type snapshot struct {
	Panes   []*pane  `json:"panes"`
	Tabs    []*tab   `json:"tabs"`
	Layouts []layout `json:"layouts"`
}
type semanticRecord struct {
	Title    string  `json:"title"`
	Terminal string  `json:"terminal_id"`
	Session  *string `json:"session_id"`
	Agent    string  `json:"agent,omitempty"`
}
type tabRecord struct {
	Original string `json:"original"`
	Last     string `json:"last"`
	Manual   bool   `json:"manual,omitempty"`
	Pane     string `json:"pane,omitempty"`
}
type titleState struct {
	Panes    map[string]string          `json:"panes"`
	Tabs     map[string]*tabRecord      `json:"tabs"`
	Semantic map[string]*semanticRecord `json:"semantic"`
}

func newState() *titleState {
	return &titleState{map[string]string{}, map[string]*tabRecord{}, map[string]*semanticRecord{}}
}
func isAgent(s string) bool { return s == "pi" }
func sessionIdentity(p *pane) string {
	if p.Session != nil && p.Session.Agent == p.Agent {
		return p.Session.Value
	}
	return ""
}
func nativeTitle(p *pane) string {
	s := cleanText(p.Native)
	if p.Agent == "pi" {
		if !strings.HasPrefix(s, "π - ") {
			return ""
		}
		s = strings.TrimPrefix(s, "π - ")
		cwd := p.Cwd
		if cwd == "" {
			cwd = p.ForegroundCwd
		}
		if cwd != "" {
			suffix := " - " + filepath.Base(cwd)
			if !strings.HasSuffix(s, suffix) {
				return ""
			}
			s = strings.TrimSuffix(s, suffix)
		} else if i := strings.LastIndex(s, " - "); i >= 0 {
			s = s[:i]
		} else {
			return ""
		}
	}
	return taskTitle(s, 32)
}
func setTitle(s snapshot, st *titleState, id, title, session, agent string) error {
	var p *pane
	for _, candidate := range s.Panes {
		if candidate.ID == id {
			p = candidate
			break
		}
	}
	if !isAgent(agent) || p == nil || p.Agent != agent {
		return fmt.Errorf("the explicit pane must contain a live %s agent", agent)
	}
	if p.Terminal == "" {
		return errors.New("pane has no terminal identity")
	}
	observed := sessionIdentity(p)
	if session != "" && observed != "" && session != observed {
		return fmt.Errorf("%s session does not match the target pane", agent)
	}
	emptyPi := agent == "pi" && title == ""
	title = taskTitle(title, 32)
	if title == "" && !emptyPi {
		return errors.New("task title must not be empty")
	}
	if session == "" {
		session = observed
	}
	var bound *string
	if session != "" {
		bound = &session
	}
	if st.Semantic == nil {
		st.Semantic = map[string]*semanticRecord{}
	}
	st.Semantic[id] = &semanticRecord{title, p.Terminal, bound, agent}
	return nil
}
func semanticTitles(panes map[string]*pane, st *titleState) map[string]string {
	if st.Semantic == nil {
		st.Semantic = map[string]*semanticRecord{}
	}
	for id, r := range st.Semantic {
		p := panes[id]
		a := r.Agent
		if !isAgent(a) || p == nil || p.Agent != a || p.Terminal != r.Terminal {
			delete(st.Semantic, id)
			continue
		}
		observed := sessionIdentity(p)
		if observed != "" && r.Session != nil && *r.Session != observed {
			delete(st.Semantic, id)
		} else if observed != "" && (r.Session == nil || *r.Session == "") {
			r.Session = &observed
		}
	}
	out := map[string]string{}
	for id, p := range panes {
		if isAgent(p.Agent) {
			if r, ok := st.Semantic[id]; ok {
				out[id] = taskTitle(r.Title, 32)
			} else {
				out[id] = nativeTitle(p)
			}
		}
	}
	return out
}

type herdrCommand func(string, map[string]any) error

func synchronize(s snapshot, st *titleState, command herdrCommand, save func() error) error {
	panes := map[string]*pane{}
	tabs := map[string]*tab{}
	focused := map[string]string{}
	for _, p := range s.Panes {
		panes[p.ID] = p
	}
	for _, t := range s.Tabs {
		tabs[t.ID] = t
	}
	for _, l := range s.Layouts {
		focused[l.Tab] = l.Focus
	}
	for id := range st.Panes {
		if panes[id] == nil {
			delete(st.Panes, id)
		}
	}
	for id := range st.Tabs {
		if tabs[id] == nil {
			delete(st.Tabs, id)
		}
	}
	desiredTitles := semanticTitles(panes, st)
	for _, p := range s.Panes {
		previous, owned := st.Panes[p.ID]
		title := desiredTitles[p.ID]
		if p.Label != "" || (p.Title != "" && p.Title != previous) {
			title = ""
		}
		params := map[string]any{"pane_id": p.ID, "source": titleSource, "seq": time.Now().UnixNano()}
		if title == "" {
			if owned {
				params["clear_title"] = true
				if err := command("pane.report_metadata", params); err != nil {
					return err
				}
				delete(st.Panes, p.ID)
				if err := save(); err != nil {
					return err
				}
			}
			continue
		}
		if p.Title != title {
			params["agent"] = p.Agent
			params["title"] = title
			if err := command("pane.report_metadata", params); err != nil {
				return err
			}
			st.Panes[p.ID] = title
			if err := save(); err != nil {
				return err
			}
		}
	}
	for _, t := range s.Tabs {
		r := st.Tabs[t.ID]
		label := t.Label
		if r != nil && label != r.Last {
			r.Manual = true
		}
		if r != nil && r.Manual {
			continue
		}
		if r == nil && !automaticLabel(label) {
			continue
		}
		var named []*pane
		count := 0
		for _, p := range s.Panes {
			if p.Tab == t.ID && isAgent(p.Agent) {
				count++
				if desiredTitles[p.ID] != "" {
					named = append(named, p)
				}
			}
		}
		if len(named) == 0 {
			if r != nil && label != r.Original {
				if err := command("tab.rename", map[string]any{"tab_id": t.ID, "label": r.Original}); err != nil {
					return err
				}
				r.Last = r.Original
				if err := save(); err != nil {
					return err
				}
			}
			continue
		}
		var chosen *pane
		for _, p := range named {
			if p.ID == focused[t.ID] {
				chosen = p
				break
			}
		}
		if chosen == nil && r != nil {
			for _, p := range named {
				if p.ID == r.Pane {
					chosen = p
					break
				}
			}
		}
		if chosen == nil {
			chosen = named[0]
		}
		suffix := ""
		if count > 1 {
			suffix = fmt.Sprintf(" · %d", count)
		}
		desired := shorten(desiredTitles[chosen.ID], 38-len([]rune(suffix))) + suffix
		if r == nil {
			r = &tabRecord{Original: label, Last: label}
			st.Tabs[t.ID] = r
		}
		r.Pane = chosen.ID
		if desired != label {
			if err := command("tab.rename", map[string]any{"tab_id": t.ID, "label": desired}); err != nil {
				return err
			}
			r.Last = desired
			if err := save(); err != nil {
				return err
			}
		}
	}
	return save()
}
func automaticLabel(s string) bool {
	switch s {
	case "zsh", "nu", "bash", "fish", "pi":
		return true
	}
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.Is(unicode.Nd, r) {
			return false
		}
	}
	return true
}
func decodeState(data []byte) (*titleState, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, errors.New("invalid task title state")
	}
	st := newState()
	if raw == nil {
		return nil, errors.New("invalid task title state")
	}
	for _, k := range []string{"panes", "tabs"} {
		b := raw[k]
		if len(b) == 0 || b[0] != '{' {
			return nil, errors.New("invalid task title state")
		}
	}
	if b, ok := raw["semantic"]; ok && (len(b) == 0 || b[0] != '{') {
		return nil, errors.New("invalid semantic title state")
	}
	if err := json.Unmarshal(data, st); err != nil {
		return nil, errors.New("invalid task title state")
	}
	for _, r := range st.Tabs {
		if r == nil {
			return nil, errors.New("invalid task title state")
		}
	}
	for _, r := range st.Semantic {
		if r == nil {
			return nil, errors.New("invalid semantic title state")
		}
	}
	return st, nil
}
