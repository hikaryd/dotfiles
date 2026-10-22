package opencode

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// readSSE bounds one event, not the entire long-lived response. raw preserves
// successful events (including id/retry/comments); errors never escape upstream.
func readSSE(r io.Reader, consume func(event, data, raw string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var lines, data []string
	event, size := "", 0
	dispatch := func() error {
		if len(lines) == 0 {
			return nil
		}
		err := consume(event, strings.Join(data, "\n"), strings.Join(lines, "\n")+"\n\n")
		lines, data, event, size = nil, nil, "", 0
		return err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := dispatch(); errors.Is(err, io.EOF) {
				return nil
			} else if err != nil {
				return err
			}
			continue
		}
		size += len(line) + 1
		if size > 1<<20 {
			return errStream
		}
		lines = append(lines, line)
		key, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch key {
		case "event":
			event = value
		case "data":
			data = append(data, value)
		}
	}
	if scanner.Err() != nil {
		return errStream
	}
	if err := dispatch(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func relaySSE(w http.ResponseWriter, r io.Reader) error {
	complete := false
	err := readSSE(r, func(event, data, raw string) error {
		if event == "error" {
			return errStream
		}
		if data != "" && data != "[DONE]" {
			var payload struct{ Error json.RawMessage }
			if json.Unmarshal([]byte(data), &payload) != nil || len(payload.Error) > 0 {
				return errStream
			}
		}
		if err := writeChunk(w, []byte(raw)); err != nil {
			return err
		}
		if data == "[DONE]" {
			complete = true
			return io.EOF
		}
		return nil
	})
	if err == nil && !complete {
		return errStream
	}
	return err
}
