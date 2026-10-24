package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	switch filepath.Base(os.Args[0]) {
	case "defaults":
		os.Exit(fakeDefaults())
	case "npm":
		fmt.Fprintln(os.Stderr, "fixture-secret-install-error")
		os.Exit(17)
	}
	os.Exit(m.Run())
}
func fakeCommand(t *testing.T, name string) {
	t.Helper()
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	bin := filepath.Join(home(), "bin")
	if e = os.MkdirAll(bin, 0700); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(bin, name)
	if e = os.Symlink(binary, p); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", bin)
}
