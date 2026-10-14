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
	pi := expand(envDefault([]string{"PI_CODING_AGENT_DIR"}, filepath.Join(home(), ".pi/agent")))
	var changes []change
	for _, p := range []string{filepath.Join(pi, "AGENTS.md")} {
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
	for _, base := range []string{filepath.Join(home(), ".agents/skills")} {
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
	fmt.Println("Jevgrep: global skill and Pi instructions installed; credentials unchanged")
	return nil
}
