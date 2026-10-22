package logwatch

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlainFilterBoundsPendingRecord(t *testing.T) {
	w := &plainFilter{output: io.Discard, terms: []string{"needle"}}
	for i := 0; i < 257; i++ {
		if _, err := w.Write([]byte(strings.Repeat("x", 65536))); err != nil {
			return
		}
	}
	t.Fatal("unbounded pending line accepted")
}
func TestDisplayQueueBoundsEachRecord(t *testing.T) {
	ch := make(chan logLine, 1)
	sendLog(context.Background(), ch, "pod", strings.Repeat("x", 1<<20))
	if len((<-ch).text) > 8192 {
		t.Fatal("oversized UI record queued")
	}
}
func TestFollowerDrainsOversizedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubectl")
	script := "#!/bin/sh\nhead -c 17825792 /dev/zero | tr '\\000' x\nprintf '\\nafter\\n'\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	output := make(chan logLine, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); followPodLogs(ctx, config{kubectl: path}, "pod", output) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("follower wedged on oversized line")
	}
	close(output)
	after := false
	for line := range output {
		if line.text == "after" {
			after = true
		}
		if len(line.text) > 8192 {
			t.Fatal("oversized record")
		}
	}
	if !after {
		t.Fatal("record after oversized line lost")
	}
}
