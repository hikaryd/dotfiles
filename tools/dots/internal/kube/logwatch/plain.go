package logwatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
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
			childCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			command := exec.CommandContext(childCtx, cfg.kubectl, logArgs(cfg, pod)...)
			command.WaitDelay = 2 * time.Second
			writer := &plainFilter{output: output, cancel: cancel}
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
	cancel  context.CancelFunc
}

func (w *plainFilter) Write(data []byte) (n int, err error) {
	defer func() {
		if err != nil && w.cancel != nil {
			w.cancel()
		}
	}()
	if len(w.terms) == 0 {
		return w.output.Write(data)
	}
	size := len(data)
	for len(data) > 0 {
		end := bytes.IndexByte(data, '\n')
		take := len(data)
		if end >= 0 {
			take = end + 1
		}
		if len(w.pending)+take > 16<<20 {
			return 0, errors.New("filtered log record exceeds 16 MiB")
		}
		w.pending = append(w.pending, data[:take]...)
		data = data[take:]
		if end < 0 {
			break
		}
		if matchesAll(string(w.pending), w.terms) {
			if _, err := w.output.Write(w.pending); err != nil {
				return 0, err
			}
		}
		w.pending = w.pending[:0]
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
