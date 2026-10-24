// Package anime presents the original anicli-api through dots' shared terminal UI.
package anime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Item struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Year        int             `json:"year"`
	Kind        string          `json:"kind"`
	Count       int             `json:"count"`
	LastEpisode string          `json:"last_episode"`
	Data        json.RawMessage `json:"data"`
}

type Episode struct {
	Title   string          `json:"title"`
	Ordinal int             `json:"ordinal"`
	Data    json.RawMessage `json:"data"`
}

func (e Episode) label() string { return fmt.Sprintf("%d %s", e.Ordinal, e.Title) }

type Video struct {
	Quality int               `json:"quality"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type service interface {
	Call(context.Context, string, any, any) error
}
type bridge struct{ python, script string }

func newBridge(root string) (bridge, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "uv", "tool", "dir").Output()
	if err != nil {
		return bridge{}, fmt.Errorf("anicli runtime unavailable; run ./install -c steps/anicli.yml: %w", err)
	}
	python := filepath.Join(strings.TrimSpace(string(output)), "anicli-ru", "bin", "python")
	if info, err := os.Stat(python); err != nil || info.IsDir() {
		return bridge{}, fmt.Errorf("anicli Python missing; run ./install -c steps/anicli.yml")
	}
	return bridge{python: python, script: filepath.Join(root, "config/anicli/backend.py")}, nil
}

// limitedBuffer bounds both API responses and subprocess diagnostics.
type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *limitedBuffer) Bytes() []byte { return b.buffer.Bytes() }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > b.limit {
		return 0, fmt.Errorf("backend output exceeded limit")
	}
	return b.buffer.Write(p)
}

// Diagnostics are bounded in memory but always drained. Returning a write
// error here would close the pipe and kill a healthy verbose player via SIGPIPE.
type diagnosticBuffer struct{ limitedBuffer }

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	keep := min(len(p), max(0, b.limit-b.buffer.Len()))
	_, _ = b.buffer.Write(p[:keep])
	return len(p), nil
}

func (b bridge) Call(ctx context.Context, op string, input, output any) error {
	request := map[string]any{}
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(data, &request); err != nil {
			return err
		}
	}
	request["op"] = op
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, b.python, b.script)
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	cmd.Stdin = bytes.NewReader(data)
	stdout := &limitedBuffer{limit: 8 << 20}
	stderr := &diagnosticBuffer{limitedBuffer: limitedBuffer{limit: 8192}}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.WaitDelay = time.Second
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var envelope struct {
		Error string `json:"error"`
	}
	decodeErr := json.Unmarshal(stdout.Bytes(), &envelope)
	if envelope.Error != "" {
		return fmt.Errorf("%s", envelope.Error)
	}
	if runErr != nil {
		return fmt.Errorf("backend failed: %w", runErr)
	}
	if decodeErr != nil {
		return fmt.Errorf("invalid response from anicli backend: %w", decodeErr)
	}
	if output == nil {
		return nil
	}
	if err := json.Unmarshal(stdout.Bytes(), output); err != nil {
		return fmt.Errorf("invalid response from anicli backend: %w", err)
	}
	return nil
}
