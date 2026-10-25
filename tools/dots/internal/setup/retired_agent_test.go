package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJevgrepInstallDoesNotRecreateRemovedPiProfile(t *testing.T) {
	h, root := jevgrepFixture(t)
	if code, _, diagnostic := runCaptured(t, root, "jevgrep-apply"); code != 0 {
		t.Fatalf("install shared Jevgrep skill: %d %s", code, diagnostic)
	}
	if _, err := os.Lstat(filepath.Join(h, ".pi")); !os.IsNotExist(err) {
		t.Fatalf("shared tool recreated removed agent profile: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(h, ".agents/skills/jevgrep")); err != nil || !sameSource(target, filepath.Join(root, "config/jevgrep/skill")) {
		t.Fatalf("shared skill not installed: %q %v", target, err)
	}
}
