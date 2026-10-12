package setup

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCommitRollsBackPublishedFilesAndLinks(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	put(t, first, "original first")
	put(t, second, "original second")
	backup := filepath.Join(dir, "occupied-backup")
	put(t, backup, "do not overwrite")
	if e := commit([]change{{path: first, data: []byte("new first")}, {path: second, data: []byte("new second"), backup: backup}}); e == nil {
		t.Fatal("expected backup collision")
	}
	if get(t, first) != "original first" || get(t, second) != "original second" || get(t, backup) != "do not overwrite" {
		t.Fatal("rollback failed")
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) != 3 {
		t.Fatal("staging files left", entries, e)
	}
}
func TestCommitRejectsStaleSourceBeforeMutation(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	put(t, first, "first")
	put(t, second, "modified source")
	before := tree(t, dir)
	e := commit([]change{{path: first, data: []byte("changed first")}, {path: second, data: []byte("reviewed patch"), expect: true, expected: []byte("reviewed source")}})
	if e == nil || !reflect.DeepEqual(before, tree(t, dir)) {
		t.Fatal("stale source not rejected atomically", e)
	}
}
func TestCommitRejectsDuplicateDestinations(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file")
	put(t, p, "original")
	e := commit([]change{{path: p, data: []byte("one")}, {path: p, data: []byte("two")}})
	if e == nil {
		t.Fatal("duplicate target accepted")
	}
	if get(t, p) != "original" {
		t.Fatal("duplicate target changed file")
	}
}
