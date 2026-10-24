package anime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestChooseVideo(t *testing.T) {
	videos := []Video{{Quality: 1080}, {Quality: 480}, {Quality: 720}}
	for requested, want := range map[int]int{0: 1080, 720: 720, 900: 720, 240: 480} {
		got, err := chooseVideo(videos, requested)
		if err != nil || got.Quality != want {
			t.Fatalf("quality %d: %#v %v", requested, got, err)
		}
	}
	if _, err := chooseVideo(nil, 720); err == nil {
		t.Fatal("empty streams accepted")
	}
}

func fakeMPV(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mpv"), []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	log := filepath.Join(dir, "args")
	t.Setenv("CALL_LOG", log)
	return log
}

func TestPlayerKeepsArgumentsLiteralAndRemembersAfterStart(t *testing.T) {
	log := fakeMPV(t, "printf '%s\\n' \"$@\" > \"$CALL_LOG\"\n")
	started := 0
	err := playVideo(context.Background(), Video{URL: "https://example.com/video.m3u8", Headers: map[string]string{"Referer": "https://example.com/a,b"}}, []string{"--vo=null", "--title=$(touch never)"}, "Фрирен\x1b[31m", func() error { started++; return nil })
	if err != nil || started != 1 {
		t.Fatalf("play: %v, started %d", err, started)
	}
	data, _ := os.ReadFile(log)
	text := string(data)
	if !strings.Contains(text, "--vo=null\n--title=$(touch never)\n") || !strings.Contains(text, "\n--\nhttps://example.com/video.m3u8\n") || strings.Contains(text, "\x1b") {
		t.Fatal("bad arguments", text)
	}
	if !strings.Contains(text, "Referer: https://example.com/a\\,b") {
		t.Fatal("unescaped headers", text)
	}
}

func TestPlayerRejectsUnsafeStreamsBeforeStarting(t *testing.T) {
	log := fakeMPV(t, "printf started > \"$CALL_LOG\"\n")
	for _, v := range []Video{{URL: "file:///etc/passwd"}, {URL: "--script=bad"}, {URL: "https://user:secret@example.com/a"}, {URL: "https://example.com/a\n"}, {URL: "https://example.com/a", Headers: map[string]string{"X-Test": "a\r\nb"}}} {
		if err := playVideo(context.Background(), v, nil, "test", func() error { t.Fatal("remembered unsafe stream"); return nil }); err == nil {
			t.Fatal("unsafe stream accepted", v)
		}
	}
	if _, err := os.Stat(log); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("player started", err)
	}
}

func TestPlayerFailureAndCancellation(t *testing.T) {
	t.Run("missing player", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if err := playVideo(context.Background(), Video{URL: "https://example.com/a"}, nil, "test", func() error { t.Fatal("remembered without player"); return nil }); err == nil {
			t.Fatal("missing player accepted")
		}
	})
	t.Run("exit failure", func(t *testing.T) {
		fakeMPV(t, "exit 31\n")
		if err := playVideo(context.Background(), Video{URL: "https://example.com/a"}, nil, "test", func() error { return nil }); err == nil {
			t.Fatal("exit failure swallowed")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		fakeMPV(t, "exec /bin/sleep 30\n")
		ctx, cancel := context.WithCancel(context.Background())
		start := time.Now()
		err := playVideo(ctx, Video{URL: "https://example.com/a"}, nil, "test", func() error { cancel(); return nil })
		if !errors.Is(err, context.Canceled) || time.Since(start) > 2*time.Second {
			t.Fatalf("cancel: %v", err)
		}
	})
	t.Run("history failure", func(t *testing.T) {
		fakeMPV(t, "exec /bin/sleep 30\n")
		want := errors.New("history unwritable")
		start := time.Now()
		err := playVideo(context.Background(), Video{URL: "https://example.com/a"}, nil, "test", func() error { return want })
		if !errors.Is(err, want) || time.Since(start) > 2*time.Second {
			t.Fatalf("history: %v", err)
		}
	})
}

func TestVerbosePlayerDoesNotFailWhenDiagnosticsAreTruncated(t *testing.T) {
	fakeMPV(t, "i=0\nwhile [ \"$i\" -lt 2000 ]; do\n  printf '%0100d\\n' 0 >&2 || exit 72\n  i=$((i+1))\ndone\n")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := playVideo(ctx, Video{URL: "https://example.com/a"}, nil, "test", func() error { return nil }); err != nil {
		t.Fatalf("verbose but successful player failed: %v", err)
	}
}
