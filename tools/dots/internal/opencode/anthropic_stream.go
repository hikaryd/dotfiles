package opencode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

var errStream = errors.New("opencode: invalid or interrupted upstream stream")

func sseEvent(w http.ResponseWriter, event string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeChunk(w, []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, b)))
}

type streamTool struct {
	id, name string
	args     strings.Builder
}

func streamAnthropic(w http.ResponseWriter, r io.Reader, model string) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	emit := func(event string, v map[string]any) error { v["type"] = event; return sseEvent(w, event, v) }
	if err := emit("message_start", map[string]any{"message": map[string]any{"id": ocID("msg"), "type": "message", "role": "assistant", "model": model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]int{"input_tokens": 0, "output_tokens": 0}}}); err != nil {
		return err
	}
	tools := map[int]*streamTool{}
	next := 0
	textIndex := -1
	finish := ""
	usage := chatUsage{}
	done := false
	pendingBytes := 0
	process := func(data string) error {
		if data == "[DONE]" {
			done = true
			return nil
		}
		var chunk struct {
			Error   json.RawMessage
			Choices []struct {
				Delta struct {
					Content *string
					Tools   []struct {
						Index    int
						ID       string
						Function function
					} `json:"tool_calls"`
				}
				Finish string `json:"finish_reason"`
			}
			Usage *chatUsage
		}
		if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Error) > 0 {
			return errStream
		}
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != nil && *choice.Delta.Content != "" {
				if textIndex < 0 {
					textIndex = next
					next++
					if err := emit("content_block_start", map[string]any{"index": textIndex, "content_block": map[string]any{"type": "text", "text": ""}}); err != nil {
						return err
					}
				}
				if err := emit("content_block_delta", map[string]any{"index": textIndex, "delta": map[string]any{"type": "text_delta", "text": *choice.Delta.Content}}); err != nil {
					return err
				}
			}
			for _, delta := range choice.Delta.Tools {
				if delta.Index < 0 || delta.Index > 1024 {
					return errStream
				}
				tool := tools[delta.Index]
				if tool == nil {
					tool = &streamTool{}
					tools[delta.Index] = tool
				}
				if len(delta.ID) > 1024 || len(tool.name)+len(delta.Function.Name) > 1024 {
					return errStream
				}
				if delta.ID != "" {
					if tool.id != "" && tool.id != delta.ID {
						return errStream
					}
					tool.id = delta.ID
				}
				if delta.Function.Name != "" {
					tool.name += delta.Function.Name
				}
				pendingBytes += len(delta.Function.Arguments)
				if pendingBytes > maxBody {
					return errStream
				}
				tool.args.WriteString(delta.Function.Arguments)
			}
			if choice.Finish != "" {
				finish = choice.Finish
			}
		}
		return nil
	}
	if err := readSSE(r, func(event, data, raw string) error {
		if data == "" {
			return nil
		}
		if event == "error" {
			return errStream
		}
		if err := process(data); err != nil {
			return err
		}
		if done {
			return io.EOF
		}
		return nil
	}); err != nil {
		return err
	}
	if finish == "" {
		return errStream
	}
	// OpenAI interleaves tools, but Anthropic SDK callbacks require exactly
	// one active block. Keep text incremental; serialize bounded tool payloads.
	if textIndex >= 0 {
		if err := emit("content_block_stop", map[string]any{"index": textIndex}); err != nil {
			return err
		}
	}
	order := make([]int, 0, len(tools))
	for index := range tools {
		order = append(order, index)
	}
	sort.Ints(order)
	for _, index := range order {
		tool := tools[index]
		if tool.id == "" || tool.name == "" {
			return errStream
		}
		args := tool.args.String()
		if args == "" {
			args = "{}"
		}
		var value map[string]json.RawMessage
		if json.Unmarshal([]byte(args), &value) != nil || value == nil {
			return errStream
		}
		if err := emit("content_block_start", map[string]any{"index": next, "content_block": map[string]any{"type": "tool_use", "id": tool.id, "name": tool.name, "input": map[string]any{}}}); err != nil {
			return err
		}
		if err := emit("content_block_delta", map[string]any{"index": next, "delta": map[string]any{"type": "input_json_delta", "partial_json": args}}); err != nil {
			return err
		}
		if err := emit("content_block_stop", map[string]any{"index": next}); err != nil {
			return err
		}
		next++
	}
	if err := emit("message_delta", map[string]any{"delta": map[string]any{"stop_reason": stopReason(finish), "stop_sequence": nil}, "usage": map[string]int{"input_tokens": usage.Input, "output_tokens": usage.Output}}); err != nil {
		return err
	}
	return emit("message_stop", map[string]any{})
}
