package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func jevgrepApply(root string) error {
	settings, e := os.ReadFile(filepath.Join(root, "config/jevgrep/instructions.md"))
	if e != nil {
		return e
	}
	source, e := resolve(filepath.Join(root, "config/jevgrep/skill"))
	if e != nil {
		return e
	}
	if _, e = os.ReadFile(filepath.Join(source, "SKILL.md")); e != nil {
		return e
	}
	claude := expand(envDefault([]string{"CLAUDE_CONFIG_DIR"}, filepath.Join(home(), ".claude")))
	pi := expand(envDefault([]string{"PI_CODING_AGENT_DIR"}, filepath.Join(home(), ".pi/agent")))
	config := expand(envDefault([]string{"XDG_CONFIG_HOME"}, filepath.Join(home(), ".config")))
	opencode := filepath.Join(config, "opencode")
	var changes []change
	for _, p := range []string{
		filepath.Join(codexDir(), "AGENTS.md"),
		filepath.Join(claude, "CLAUDE.md"),
		filepath.Join(pi, "AGENTS.md"),
		filepath.Join(opencode, "AGENTS.md"),
	} {
		b, e := regularOptional(p)
		if e != nil {
			return e
		}
		s, e := instructionBlock(string(b), string(settings), "<!-- BEGIN DOTS SEARCH -->", "<!-- END DOTS SEARCH -->")
		if e != nil {
			return e
		}
		changes = append(changes, change{path: p, data: []byte(s), expect: true, expected: b,
			backup: privateBackup("jevgrep", p), backupPrivate: true})
	}
	for _, base := range []string{
		filepath.Join(home(), ".agents/skills"),
		filepath.Join(codexDir(), "skills"),
		filepath.Join(claude, "skills"),
		filepath.Join(opencode, "skills"),
	} {
		p := filepath.Join(base, "jevgrep")
		if _, e := os.Lstat(p); e == nil {
			if !sameSource(p, source) {
				return errors.New("existing Jevgrep skill preserved; move it aside before installing")
			}
		} else if !os.IsNotExist(e) {
			return e
		}
		changes = append(changes, change{path: p, link: source})
	}
	if e = commit(changes); e != nil {
		return e
	}
	fmt.Println("Jevgrep: global skill and Codex/Claude/Pi/OpenCode instructions installed; credentials unchanged")
	return nil
}
