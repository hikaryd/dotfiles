package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

func runLogFixture() {
	if strings.Contains(" "+strings.Join(os.Args[1:], " ")+" ", " get namespaces ") {
		fmt.Println(`{"items":[{"metadata":{"name":"default"}},{"metadata":{"name":"application"}}]}`)
		return
	}
	if strings.Contains(" "+strings.Join(os.Args[1:], " ")+" ", " get pods ") {
		fmt.Println(`{"items":[{"metadata":{"name":"api","creationTimestamp":"2026-01-01T00:00:00Z"},"status":{"phase":"Running","containerStatuses":[{"name":"app","ready":true}]}}]}`)
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	pod := "api"
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "pod/") {
			pod = strings.TrimPrefix(arg, "pod/")
		}
	}
	output := bufio.NewWriterSize(os.Stdout, 64*1024)
	if os.Getenv("DOTS_KUBE_LOG_JSON") == "1" {
		fmt.Fprintln(output, "[pod/"+pod+"/app] "+`{"level":"INFO","message":"readable first line\nreadable second line","hidden_detail":"DETAIL_SENTINEL","payload":{"one":1,"two":2}}`)
		output.Flush()
		<-signals
		return
	}
	if os.Getenv("DOTS_KUBE_LOG_BURST") == "1" {
		for i := 0; i < 10000; i++ {
			fmt.Fprintf(output, "[pod/%s/app] INFO replay-%08d request finished\n", pod, i)
		}
		fmt.Fprintf(output, "[pod/%s/app] REPLAY_DONE\n", pod)
		output.Flush()
		<-signals
		return
	}
	counter := 0
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-signals:
			return
		case <-ticker.C:
			for i := 0; i < 200; i++ {
				fmt.Fprintf(output, "[pod/%s/app] INFO record-%08d request finished\n", pod, counter)
				counter++
			}
			output.Flush()
		}
	}
}
func BenchmarkLogReplay(b *testing.B) {
	home := b.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		b.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		b.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(bin, "kubectl")); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		command := exec.CommandContext(ctx, testBinary, "kube-log-watch", "--context", "dots-stage", "--namespace", "app", "--environment", "stage", "--pod", "api", "--buffer", "10000")
		command.Env = []string{"HOME=" + home, "PATH=" + bin, "DOTS_KUBE_LOG_FIXTURE=1", "DOTS_KUBE_LOG_BURST=1", "NO_COLOR=1", "TERM=xterm-256color", "GORACE=atexit_sleep_ms=0"}
		terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 24, Cols: 90})
		if err != nil {
			cancel()
			b.Fatal(err)
		}
		capture := &terminalCapture{}
		go func() {
			buffer := make([]byte, 32768)
			for {
				n, err := terminal.Read(buffer)
				if n > 0 {
					capture.Write(buffer[:n])
				}
				if err != nil {
					return
				}
			}
		}()
		for !strings.Contains(capture.String(), "REPLAY_DONE") && ctx.Err() == nil {
			time.Sleep(10 * time.Millisecond)
		}
		if ctx.Err() != nil {
			command.Process.Kill()
			command.Wait()
			terminal.Close()
			cancel()
			b.Fatal("replay timed out")
		}
		terminal.Write([]byte("q"))
		if err := command.Wait(); err != nil {
			b.Fatal(err)
		}
		terminal.Close()
		cancel()
	}
}

func TestHotLogsDoNotRepaintStaticChromeAndRemainControllable(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(bin, "kubectl")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, testBinary, "kube-log-watch", "--context", "dots-stage", "--namespace", "app", "--environment", "stage", "--pod", "api", "--pod", "worker", "--buffer", "5000")
	command.Env = []string{"HOME=" + home, "PATH=" + bin, "DOTS_KUBE_LOG_FIXTURE=1", "NO_COLOR=1", "TERM=xterm-256color", "GORACE=atexit_sleep_ms=0"}
	terminal, err := pty.StartWithSize(command, &pty.Winsize{Rows: 24, Cols: 90})
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	defer command.Process.Kill()
	capture := &terminalCapture{}
	go func() {
		buffer := make([]byte, 32768)
		for {
			n, err := terminal.Read(buffer)
			if n > 0 {
				capture.Write(buffer[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	awaitScreen(t, capture, "record-")
	time.Sleep(1200 * time.Millisecond)
	before := capture.String()
	headers := strings.Count(before, "context: dots-stage")
	if headers > 2 {
		t.Errorf("static chrome was repainted %d times under load; bytes=%d", headers, len(before))
	}
	if strings.Contains(before, "\x1b[2J") || strings.Contains(before, "\x1b[H\x1b[2J") {
		t.Error("full-screen clear while following logs")
	}
	terminal.Write([]byte(" "))
	awaitScreen(t, capture, "PAUSED")
	pausedBytes := len(capture.String())
	time.Sleep(350 * time.Millisecond)
	if strings.Contains(capture.String()[pausedBytes:], "record-") {
		t.Error("paused records changed under load")
	}
	terminal.Write([]byte("k"))
	time.Sleep(100 * time.Millisecond)
	terminal.Write([]byte("q"))
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("quit %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("keyboard is unresponsive under log load")
	}
}
