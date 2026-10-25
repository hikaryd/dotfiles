package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDevProjectsScanIsShallow(t *testing.T) {
	h := t.TempDir()
	t.Setenv("HOME", h)
	root := filepath.Join(h, "Documents/dev")
	for _, name := range []string{"Alpha/Not a separate project", "Beta 界", ".cache"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	projects, err := devProjects()
	if err != nil || len(projects) != 2 || projects[0].Name != "Alpha" || projects[1].Name != "Beta 界" {
		t.Fatalf("project catalog: %+v %v", projects, err)
	}
}
