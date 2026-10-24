package deploywatch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogControlsAndRecordBound(t *testing.T) {
	d := &dashboard{logLines: make(chan logLine, 1)}
	d.sendLog(context.Background(), "pod", "ok\x1b[2J\x1b]52;c;clipboard\a")
	if strings.ContainsRune((<-d.logLines).text, '\x1b') {
		t.Fatal("terminal command survived")
	}
	d.sendLog(context.Background(), "pod", strings.Repeat("x", 1<<20))
	if len((<-d.logLines).text) > 8192 {
		t.Fatal("oversized UI record queued")
	}
}
func TestCompletedStreamsAllReconnect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubectl")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	d := &dashboard{cfg: config{kubectl: path}, streams: map[string]logStream{}, logLines: make(chan logLine, 256)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer d.stopStreams()
	pods := make([]pod, 40)
	for i := range pods {
		pods[i].Metadata.Name = fmt.Sprintf("pod-%d", i)
	}
	d.syncLogStreams(ctx, pods)
	before := map[string]<-chan struct{}{}
	for name, s := range d.streams {
		before[name] = s.done
		select {
		case <-s.done:
		case <-ctx.Done():
			t.Fatal("follower did not finish")
		}
	}
	d.syncLogStreams(ctx, pods)
	for name, s := range d.streams {
		if s.done == before[name] {
			t.Fatalf("stream %s was not replaced", name)
		}
		select {
		case <-s.done:
		case <-ctx.Done():
			t.Fatal("replacement did not finish")
		}
	}
}
func TestFollowerDrainsOversizedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubectl")
	script := "#!/bin/sh\nhead -c 17825792 /dev/zero | tr '\\000' x\nprintf '\\nafter\\n'\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	d := &dashboard{cfg: config{kubectl: path}, logLines: make(chan logLine, 8)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); d.followLogs(ctx, "pod") }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("follower wedged on oversized line")
	}
	close(d.logLines)
	after := false
	for line := range d.logLines {
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
