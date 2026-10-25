package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func jevgrepApply(root string) error {
	source, e := resolve(filepath.Join(root, "config/jevgrep/skill"))
	if e != nil {
		return e
	}
	if _, e = os.ReadFile(filepath.Join(source, "SKILL.md")); e != nil {
		return e
	}
	var changes []change
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
	fmt.Println("Jevgrep: global skill installed; credentials unchanged")
	return nil
}
