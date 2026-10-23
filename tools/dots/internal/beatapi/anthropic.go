package beatapi

import (
	"encoding/json"
	"errors"
	"strings"
)

var errAnthropic = errors.New("beatapi: unsupported or malformed Anthropic content; only text and tools are supported")

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}
type function struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type toolCall struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Function function `json:"function"`
}
type chatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

func textContent(raw json.RawMessage) (string, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil || blocks == nil {
		return "", errAnthropic
	}
	var b strings.Builder
	for _, v := range blocks {
		if v.Type != "text" {
			return "", errAnthropic
		}
		b.WriteString(v.Text)
	}
	return b.String(), nil
}
func anthropicRequest(body []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return nil, errAnthropic
	}
	var input struct {
		System   json.RawMessage `json:"system"`
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		Tools []struct {
			Name        string
			Description string
			Schema      json.RawMessage `json:"input_schema"`
		}
		ToolChoice *struct {
			Type, Name string
			Disable    bool `json:"disable_parallel_tool_use"`
		} `json:"tool_choice"`
	}
	if json.Unmarshal(body, &input) != nil || len(input.Messages) == 0 {
		return nil, errAnthropic
	}
	messages := []chatMessage{}
	if len(input.System) > 0 {
		text, err := textContent(input.System)
		if err != nil {
			return nil, err
		}
		messages = append(messages, chatMessage{Role: "system", Content: text})
	}
	for _, msg := range input.Messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			return nil, errAnthropic
		}
		var text string
		if json.Unmarshal(msg.Content, &text) == nil {
			messages = append(messages, chatMessage{Role: msg.Role, Content: text})
			continue
		}
		var parts []block
		if json.Unmarshal(msg.Content, &parts) != nil || parts == nil {
			return nil, errAnthropic
		}
		if msg.Role == "assistant" {
			var texts strings.Builder
			var calls []toolCall
			for _, b := range parts {
				switch b.Type {
				case "text":
					texts.WriteString(b.Text)
				case "tool_use":
					var obj map[string]json.RawMessage
					if b.ID == "" || b.Name == "" || json.Unmarshal(b.Input, &obj) != nil || obj == nil {
						return nil, errAnthropic
					}
					calls = append(calls, toolCall{ID: b.ID, Type: "function", Function: function{Name: b.Name, Arguments: string(b.Input)}})
				default:
					return nil, errAnthropic
				}
			}
			var content any
			if texts.Len() > 0 {
				content = texts.String()
			}
			messages = append(messages, chatMessage{Role: "assistant", Content: content, ToolCalls: calls})
		} else {
			for _, b := range parts {
				switch b.Type {
				case "text":
					messages = append(messages, chatMessage{Role: "user", Content: b.Text})
				case "tool_result":
					if len(b.Content) == 0 {
						b.Content = json.RawMessage(`""`)
					}
					text, err := textContent(b.Content)
					if err != nil || b.ToolUseID == "" {
						return nil, errAnthropic
					}
					messages = append(messages, chatMessage{Role: "tool", Content: text, ToolCallID: b.ToolUseID})
				default:
					return nil, errAnthropic
				}
			}
		}
	}
	out := map[string]any{"messages": messages}
	for _, k := range []string{"model", "max_tokens", "temperature", "top_p", "stream"} {
		if v, ok := fields[k]; ok {
			out[k] = v
		}
	}
	if string(fields["stream"]) == "true" {
		out["stream_options"] = map[string]bool{"include_usage": true}
	}
	if v, ok := fields["stop_sequences"]; ok {
		out["stop"] = v
	}
	if len(input.Tools) > 0 {
		tools := []any{}
		for _, tool := range input.Tools {
			if tool.Name == "" || !json.Valid(tool.Schema) {
				return nil, errAnthropic
			}
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": tool.Schema}})
		}
		out["tools"] = tools
	}
	if choice := input.ToolChoice; choice != nil {
		switch choice.Type {
		case "auto", "none":
			out["tool_choice"] = choice.Type
		case "any":
			out["tool_choice"] = "required"
		case "tool":
			if choice.Name == "" {
				return nil, errAnthropic
			}
			out["tool_choice"] = map[string]any{"type": "function", "function": map[string]string{"name": choice.Name}}
		default:
			return nil, errAnthropic
		}
		if choice.Disable {
			out["parallel_tool_calls"] = false
		}
	}
	return json.Marshal(out)
}

type chatUsage struct {
	Input  int `json:"prompt_tokens"`
	Output int `json:"completion_tokens"`
}

func stopReason(finish string) string {
	switch finish {
	case "tool_calls", "function_call":
		return "tool_use"
	case "length":
		return "max_tokens"
	case "content_filter":
		return "refusal"
	default:
		return "end_turn"
	}
}
func anthropicResponse(body []byte, model string) ([]byte, error) {
	var input struct {
		Choices []struct {
			Message struct {
				Content   json.RawMessage
				ToolCalls []toolCall `json:"tool_calls"`
			}
			Finish string `json:"finish_reason"`
		}
		Usage chatUsage
	}
	if json.Unmarshal(body, &input) != nil || len(input.Choices) == 0 {
		return nil, errors.New("beatapi: invalid upstream completion")
	}
	choice := input.Choices[0]
	content := []block{}
	if len(choice.Message.Content) > 0 && string(choice.Message.Content) != "null" {
		text, err := textContent(choice.Message.Content)
		if err != nil {
			return nil, errors.New("beatapi: unsupported upstream content")
		}
		if text != "" {
			content = append(content, block{Type: "text", Text: text})
		}
	}
	for _, tool := range choice.Message.ToolCalls {
		var obj map[string]json.RawMessage
		if tool.ID == "" || tool.Function.Name == "" || json.Unmarshal([]byte(tool.Function.Arguments), &obj) != nil || obj == nil {
			return nil, errors.New("beatapi: invalid upstream tool call")
		}
		content = append(content, block{Type: "tool_use", ID: tool.ID, Name: tool.Function.Name, Input: json.RawMessage(tool.Function.Arguments)})
	}
	return json.Marshal(map[string]any{"id": newID("msg"), "type": "message", "role": "assistant", "model": model, "content": content, "stop_reason": stopReason(choice.Finish), "stop_sequence": nil, "usage": map[string]int{"input_tokens": input.Usage.Input, "output_tokens": input.Usage.Output}})
}
