package setup

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func jevgrepFixture(t *testing.T) (string, string) {
	t.Helper()
	h := fixtureHome(t)
	root := filepath.Join(h, "root")
	put(t, filepath.Join(root, "config/jevgrep/skill/SKILL.md"), "fixture skill\n")
	return h, root
}

func TestJevgrepGlobalSkillAndIdempotence(t *testing.T) {
	h, root := jevgrepFixture(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, "custom-config"))
	credentials := filepath.Join(h, "custom-config/jevgrep/credentials.json")
	put(t, credentials, "private fixture credentials")
	if code, out, err := runCaptured(t, root, "jevgrep-apply"); code != 0 {
		t.Fatal(code, out, err)
	}
	target, err := os.Readlink(filepath.Join(h, ".agents/skills/jevgrep"))
	if err != nil || !sameSource(target, filepath.Join(root, "config/jevgrep/skill")) {
		t.Fatal(target, err)
	}
	if get(t, credentials) != "private fixture credentials" {
		t.Fatal("credentials changed")
	}
	for _, retired := range []string{".pi", ".codex", ".claude", "custom-config/opencode"} {
		if _, e := os.Lstat(filepath.Join(h, retired)); !os.IsNotExist(e) {
			t.Fatalf("installer recreated retired agent directory %s: %v", retired, e)
		}
	}
	before := tree(t, h)
	if code, out, err := runCaptured(t, root, "jevgrep-apply"); code != 0 || !reflect.DeepEqual(before, tree(t, h)) {
		t.Fatal("not idempotent", code, out, err)
	}
}

func TestJevgrepRejectsInvalidInputBeforeMutation(t *testing.T) {
	for _, scenario := range []string{"existing skill", "missing source"} {
		t.Run(scenario, func(t *testing.T) {
			h, root := jevgrepFixture(t)
			switch scenario {
			case "existing skill":
				put(t, filepath.Join(h, ".agents/skills/jevgrep/SKILL.md"), "personal skill")
			case "missing source":
				if e := os.Remove(filepath.Join(root, "config/jevgrep/skill/SKILL.md")); e != nil {
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
