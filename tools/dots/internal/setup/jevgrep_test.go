package setup

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func jevgrepFixture(t *testing.T) (string, string) {
	t.Helper()
	h := fixtureHome(t)
	root := filepath.Join(h, "root")
	put(t, filepath.Join(root, "config/jevgrep/instructions.md"), "shared search instructions\n")
	put(t, filepath.Join(root, "config/jevgrep/skill/SKILL.md"), "fixture skill\n")
	return h, root
}

func TestJevgrepGlobalInstructionsSkillAndIdempotence(t *testing.T) {
	h, root := jevgrepFixture(t)
	t.Setenv("CODEX_CONFIG_DIR", filepath.Join(h, "custom-codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(h, "custom-claude"))
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(h, "custom-pi"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, "custom-config"))
	docs := []string{"custom-codex/AGENTS.md", "custom-claude/CLAUDE.md", "custom-pi/AGENTS.md", "custom-config/opencode/AGENTS.md"}
	for _, p := range docs {
		put(t, filepath.Join(h, p), "personal rules\n<!-- BEGIN DOTS SEARCH -->\nold rules\n<!-- END DOTS SEARCH -->\ntrailing rules\n")
	}
	credentials := filepath.Join(h, "custom-config/jevgrep/credentials.json")
	put(t, credentials, "private fixture credentials")
	if code, out, err := runCaptured(t, root, "jevgrep-apply"); code != 0 {
		t.Fatal(code, out, err)
	}
	for _, p := range docs {
		s := get(t, filepath.Join(h, p))
		if !strings.Contains(s, "personal rules") || !strings.Contains(s, "trailing rules") ||
			!strings.Contains(s, "shared search instructions") || strings.Contains(s, "old rules") {
			t.Fatal(p, s)
		}
	}
	for _, p := range []string{".agents/skills/jevgrep", "custom-codex/skills/jevgrep", "custom-claude/skills/jevgrep", "custom-config/opencode/skills/jevgrep"} {
		target, err := os.Readlink(filepath.Join(h, p))
		if err != nil || !sameSource(target, filepath.Join(root, "config/jevgrep/skill")) {
			t.Fatal(p, target, err)
		}
	}
	if get(t, credentials) != "private fixture credentials" {
		t.Fatal("credentials changed")
	}
	before := tree(t, h)
	if code, out, err := runCaptured(t, root, "jevgrep-apply"); code != 0 || !reflect.DeepEqual(before, tree(t, h)) {
		t.Fatal("not idempotent", code, out, err)
	}
}

func TestJevgrepRejectsInvalidInputBeforeMutation(t *testing.T) {
	for _, scenario := range []string{"incomplete block", "existing skill", "missing source", "directory as instructions"} {
		t.Run(scenario, func(t *testing.T) {
			h, root := jevgrepFixture(t)
			switch scenario {
			case "incomplete block":
				put(t, filepath.Join(h, ".claude/CLAUDE.md"), "<!-- BEGIN DOTS SEARCH -->\nno end")
			case "existing skill":
				put(t, filepath.Join(h, ".claude/skills/jevgrep/SKILL.md"), "personal skill")
			case "missing source":
				if e := os.Remove(filepath.Join(root, "config/jevgrep/skill/SKILL.md")); e != nil {
					t.Fatal(e)
				}
			case "directory as instructions":
				if e := os.MkdirAll(filepath.Join(h, ".pi/agent/AGENTS.md"), 0700); e != nil {
					t.Fatal(e)
				}
			}
			before := tree(t, h)
			if code, _, _ := runCaptured(t, root, "jevgrep-apply"); code != 1 || !reflect.DeepEqual(before, tree(t, h)) {
				t.Fatal("invalid input changed files", code)
			}
		})
	}
}
