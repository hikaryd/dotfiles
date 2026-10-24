package beatapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslatedStreamRequestsUsage(t *testing.T) {
	raw, err := anthropicRequest([]byte(`{"stream":true,"model":"glm-5.3-free","messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Options struct {
			Usage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	json.Unmarshal(raw, &request)
	if !request.Options.Usage {
		t.Fatal("upstream usage not requested")
	}
}
func TestToolStreamAggregateBound(t *testing.T) {
	for _, size := range []int{maxBody - 16, maxBody + 16} {
		var input strings.Builder
		input.WriteString("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"find\"}}]}}]}\n\n")
		args := "{\"value\":\"" + strings.Repeat("x", size) + "\"}"
		for len(args) > 0 {
			n := min(len(args), 64<<10)
			fragment, err := json.Marshal(args[:n])
			if err != nil {
				t.Fatal(err)
			}
			input.WriteString("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":" + string(fragment) + "}}]}}]}\n\n")
			args = args[n:]
		}
		input.WriteString("data: {\"choices\":[{\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
		err := streamAnthropic(httptest.NewRecorder(), strings.NewReader(input.String()), "glm-5.3-free")
		if (err != nil) != (size > maxBody) {
			t.Fatalf("size %d: %v", size, err)
		}
	}
}
