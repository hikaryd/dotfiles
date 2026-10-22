package setup

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestInstructionBlockRejectsAmbiguousMarkers(t *testing.T) {
	const begin, end = "<!-- BEGIN DOTS SEARCH -->", "<!-- END DOTS SEARCH -->"
	for _, input := range []string{begin + "\npersonal\n" + begin + "\nold\n" + end, begin + "\nold\n" + end + "\n" + end, end, "inline " + begin + "\nold\n" + end, end + "\n" + begin} {
		if _, err := instructionBlock(input, "new", begin, end); err == nil {
			t.Errorf("accepted ambiguous markers: %q", input)
		}
	}
}

func TestWaterfoxRejectsEditsAfterPlanning(t *testing.T) {
	for _, exists := range []bool{true, false} {
		t.Run(map[bool]string{true: "edited", false: "created"}[exists], func(t *testing.T) {
			h := fixtureHome(t)
			root := filepath.Join(h, "root")
			base := filepath.Join(h, "Library/Application Support/Waterfox")
			put(t, filepath.Join(root, "config/waterfox/user.js"), "managed")
			put(t, filepath.Join(base, "profiles.ini"), "[Profile0]\nPath=first\n[Profile1]\nPath=second\n")
			for _, name := range []string{"first", "second"} {
				put(t, filepath.Join(base, name, "prefs.js"), "used")
			}
			first := filepath.Join(base, "first/user.js")
			if exists {
				put(t, first, "original")
			}
			second := filepath.Join(base, "second/user.js")
			if err := syscall.Mkfifo(second, 0600); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- waterfoxApply(root) }()
			// Opening the writer synchronizes with reading the second profile, after
			// the first profile's merge was planned, without timing-dependent sleeps.
			writer, err := os.OpenFile(second, os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.Remove(second); err != nil {
				t.Fatal(err)
			}
			put(t, second, "second original")
			put(t, first, "concurrent personal edit")
			if _, err = writer.WriteString("second original"); err != nil {
				t.Fatal(err)
			}
			writer.Close()
			if err = <-done; err == nil || !strings.Contains(err.Error(), "changed") {
				t.Errorf("wanted stale-source rejection, got %v", err)
			}
			if got := get(t, first); got != "concurrent personal edit" {
				t.Errorf("concurrent edit overwritten: %q", got)
			}
		})
	}
}
