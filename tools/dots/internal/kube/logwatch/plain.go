package logwatch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// Piped logs bypass the presentation module. Preserve kubectl bytes, separate
// stdout/stderr and report failures rather than turning them into log records.
func streamPlain(ctx context.Context, cfg config) int {
	var lock sync.Mutex
	output := lockedWriter{&lock, os.Stdout}
	diagnostic := lockedWriter{&lock, os.Stderr}
	results := make(chan error, len(cfg.pods))
	for _, pod := range cfg.pods {
		go func(pod string) {
			command := exec.CommandContext(ctx, cfg.kubectl, logArgs(cfg, pod)...)
			writer := &plainFilter{output: output}
			if cfg.filter {
				writer.terms = queryTerms(cfg.query)
			}
			command.Stdout = writer
			command.Stderr = diagnostic
			err := command.Run()
			if flushErr := writer.flush(); err == nil {
				err = flushErr
			}
			results <- err
		}(pod)
	}
	status := 0
	for range cfg.pods {
		select {
		case <-ctx.Done():
			return 130
		case err := <-results:
			if err == nil {
				continue
			}
			code := 1
			if failure, ok := err.(*exec.ExitError); ok && failure.ExitCode() > 0 {
				code = failure.ExitCode()
			} else {
				fmt.Fprintln(diagnostic, err)
			}
			if status == 0 {
				status = code
			}
		}
	}
	return status
}

type lockedWriter struct {
	lock   *sync.Mutex
	output io.Writer
}

func (w lockedWriter) Write(data []byte) (int, error) {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.output.Write(data)
}

type plainFilter struct {
	output  io.Writer
	terms   []string
	pending []byte
}

func (w *plainFilter) Write(data []byte) (int, error) {
	if len(w.terms) == 0 {
		return w.output.Write(data)
	}
	size := len(data)
	w.pending = append(w.pending, data...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			break
		}
		line := w.pending[:end+1]
		if matchesAll(string(line), w.terms) {
			if _, err := w.output.Write(line); err != nil {
				return 0, err
			}
		}
		w.pending = w.pending[end+1:]
	}
	return size, nil
}
func (w *plainFilter) flush() error {
	if len(w.pending) > 0 && matchesAll(string(w.pending), w.terms) {
		_, err := w.output.Write(w.pending)
		w.pending = nil
		return err
	}
	w.pending = nil
	return nil
}
