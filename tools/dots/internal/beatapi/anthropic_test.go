package beatapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicRequestMixedTools(t *testing.T) {
	b, err := anthropicRequest([]byte(`{"model":"big-pickle","max_tokens":128,"system":[{"type":"text","text":"system"}],"tools":[{"name":"find","description":"lookup","input_schema":{"type":"object"}}],"tool_choice":{"type":"tool","name":"find","disable_parallel_tool_use":true},"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":[{"type":"text","text":"looking"},{"type":"tool_use","id":"call-1","name":"find","input":{"q":"x"}},{"type":"tool_use","id":"call-2","name":"find","input":{"q":"y"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":[{"type":"text","text":"found"}]},{"type":"text","text":"continue"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Messages []struct {
			Role      string
			Content   any
			ToolCalls []struct {
				ID       string
				Function struct{ Name, Arguments string }
			} `json:"tool_calls"`
		}
		Tools []struct {
			Type     string
			Function struct {
				Name       string
				Parameters any
			}
		}
		Parallel bool `json:"parallel_tool_calls"`
		Max      int  `json:"max_tokens"`
	}
	if json.Unmarshal(b, &got) != nil || len(got.Messages) != 5 || got.Messages[0].Role != "system" || got.Messages[2].Content != "looking" || len(got.Messages[2].ToolCalls) != 2 || got.Messages[3].Role != "tool" || got.Messages[3].Content != "found" || got.Messages[4].Content != "continue" || got.Max != 128 || got.Parallel {
		t.Fatalf("lost conversion: %s", b)
	}
	if got.Messages[2].ToolCalls[0].Function.Arguments != `{"q":"x"}` || got.Tools[0].Function.Name != "find" {
		t.Fatal("lost tool schema/input")
	}
	for _, bad := range []string{`{"messages":[{"role":"user","content":[{"type":"image","source":{}}]}]}`, `{"messages":[{"role":"tool","content":"x"}]}`, `{"messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"image"}]}]}]}`} {
		if _, err := anthropicRequest([]byte(bad)); err == nil {
			t.Fatal("unsupported content silently discarded")
		}
	}
}
func TestAnthropicEmptyToolResult(t *testing.T) {
	b, err := anthropicRequest([]byte(`{"model":"big-pickle","messages":[{"role":"assistant","content":[{"type":"tool_use","id":"c1","name":"noop","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"c1"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Messages []chatMessage }
	if json.Unmarshal(b, &got) != nil || len(got.Messages) != 2 || got.Messages[1].Role != "tool" || got.Messages[1].Content != "" {
		t.Fatalf("empty tool result was lost: %s", b)
	}
}
func TestAnthropicToolMetadataBounded(t *testing.T) {
	chunk := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"" + strings.Repeat("x", 512) + "\"}}]}}]}\n\n"
	input := strings.Repeat(chunk, 16) + "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n"
	if err := streamAnthropic(httptest.NewRecorder(), strings.NewReader(input), "big-pickle"); err == nil {
		t.Fatal("unbounded pending tool name accepted")
	}
}
func TestAnthropicResponseToolsAndReasons(t *testing.T) {
	for _, tc := range []struct{ finish, want string }{{"tool_calls", "tool_use"}, {"length", "max_tokens"}, {"stop", "end_turn"}, {"content_filter", "refusal"}} {
		b, err := anthropicResponse([]byte(`{"id":"chat-1","choices":[{"message":{"content":"hello","tool_calls":[{"id":"c1","function":{"name":"a","arguments":"{\"x\":1}"}},{"id":"c2","function":{"name":"b","arguments":"{}"}}]},"finish_reason":"`+tc.finish+`"}],"usage":{"prompt_tokens":12,"completion_tokens":8}}`), "big-pickle")
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Content []struct {
				Type, ID, Name, Text string
				Input                map[string]any
			}
			Stop  string `json:"stop_reason"`
			Usage struct {
				Input  int `json:"input_tokens"`
				Output int `json:"output_tokens"`
			}
		}
		json.Unmarshal(b, &got)
		if len(got.Content) != 3 || got.Content[1].ID != "c1" || got.Content[2].Name != "b" || got.Stop != tc.want || got.Usage.Input != 12 || got.Usage.Output != 8 {
			t.Fatalf("bad response %s", b)
		}
	}
	if _, err := anthropicResponse([]byte(`{"choices":[{"message":{"tool_calls":[{"function":{"arguments":"not json"}}]}}]}`), "big-pickle"); err == nil {
		t.Fatal("invalid tool JSON accepted")
	}
}
func TestAnthropicStreamingInterleavedTools(t *testing.T) {
	input := `data: {"id":"chat-1","choices":[{"delta":{"content":"Hi"}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"find","arguments":"{\"q\":"}},{"index":1,"id":"c2","function":{"name":"read","arguments":"{"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"}"}},{"index":0,"function":{"arguments":"\"x\"}"}}]}}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":12,"completion_tokens":8}}

data: [DONE]

`
	w := httptest.NewRecorder()
	if err := streamAnthropic(w, strings.NewReader(input), "big-pickle"); err != nil {
		t.Fatal(err)
	}
	out := w.Body.String()
	for _, s := range []string{`event: message_start`, `"type":"text_delta"`, `"id":"c1"`, `"id":"c2"`, `"type":"input_json_delta"`, `"stop_reason":"tool_use"`, `"output_tokens":8`, `event: message_stop`} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing %s: %s", s, out)
		}
	}
	open := false
	for _, event := range strings.Split(out, "\n\n") {
		if strings.HasPrefix(event, "event: content_block_start") {
			if open {
				t.Fatal("overlapping Anthropic blocks")
			}
			open = true
		}
		if strings.HasPrefix(event, "event: content_block_stop") {
			if !open {
				t.Fatal("stop without active block")
			}
			open = false
		}
	}
	if strings.Count(out, "event: content_block_stop") != 3 || strings.Count(out, "event: message_stop") != 1 {
		t.Fatal("blocks not closed exactly once")
	}
	if err := streamAnthropic(httptest.NewRecorder(), strings.NewReader("data: invalid\n\n"), "big-pickle"); err == nil {
		t.Fatal("invalid SSE accepted")
	}
	if err := streamAnthropic(httptest.NewRecorder(), strings.NewReader("data: {\"error\":{\"message\":\"secret\"}}\n\n"), "big-pickle"); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("unsafe streaming error")
	}
}
func TestAnthropicStreamingDelayedToolArguments(t *testing.T) {
	input := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"find","arguments":""}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"c2","function":{"name":"read","arguments":"{}"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":\"x\"}"}}]},"finish_reason":"tool_calls"}]}

data: [DONE]

`
	w := httptest.NewRecorder()
	if err := streamAnthropic(w, strings.NewReader(input), "big-pickle"); err != nil {
		t.Fatal(err)
	}
	type block struct {
		id, name, args string
		stopped        bool
	}
	var blocks []block
	for _, raw := range strings.Split(w.Body.String(), "\n\n") {
		_, data, ok := strings.Cut(raw, "\ndata: ")
		if !ok {
			continue
		}
		var event struct {
			Type  string
			Index int
			Block struct{ ID, Name string } `json:"content_block"`
			Delta struct {
				JSON string `json:"partial_json"`
			}
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatal(err)
		}
		switch event.Type {
		case "content_block_start":
			if event.Index != len(blocks) {
				t.Fatalf("block started at index %d, expected %d", event.Index, len(blocks))
			}
			blocks = append(blocks, block{id: event.Block.ID, name: event.Block.Name})
		case "content_block_delta", "content_block_stop":
			if event.Index < 0 || event.Index >= len(blocks) || blocks[event.Index].stopped {
				t.Fatalf("event addresses missing or closed block: %s", raw)
			}
			if event.Type == "content_block_stop" {
				blocks[event.Index].stopped = true
			} else {
				blocks[event.Index].args += event.Delta.JSON
			}
		}
	}
	if len(blocks) != 2 || blocks[0] != (block{"c1", "find", `{"q":"x"}`, true}) || blocks[1] != (block{"c2", "read", "{}", true}) {
		t.Fatalf("cannot reconstruct interleaved tools: %+v", blocks)
	}
}
func TestMessagesRouteIntegration(t *testing.T) {
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/chat/completions" || !strings.Contains(string(b), `"role":"system"`) {
			t.Error("wrong translated request")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`)
	}))
	defer u.Close()
	h := newHandler(func() (Config, error) { return testConfig(), nil }, u.Client(), u.URL)
	w := request(h, "POST", "/v1/messages", `{"model":"glm-5.3-free","system":"test","messages":[{"role":"user","content":"hi"}]}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"type":"message"`) || !strings.Contains(w.Body.String(), `"text":"hello"`) {
		t.Fatalf("bad messages response: %d %s", w.Code, w.Body)
	}
}
