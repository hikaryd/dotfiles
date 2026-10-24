package logstream

import (
	"strings"
	"testing"
)

func TestScanDrainsOversizedRecordAndContinues(t *testing.T) {
	var lines []string
	err := Scan(strings.NewReader(strings.Repeat("a", 17<<20)+"\nnext\nfinal"), func(s string) { lines = append(lines, s) })
	if err != nil || len(lines) != 3 || len(lines[0]) > MaxLineBytes || lines[1] != "next" || lines[2] != "final" {
		t.Fatalf("count=%d err=%v", len(lines), err)
	}
}
func TestTextRemovesTerminalPayloads(t *testing.T) {
	if got := Text("ok\x1b]52;c;clipboard\a\x1b[31mred"); got != "okred" {
		t.Fatalf("%q", got)
	}
}
