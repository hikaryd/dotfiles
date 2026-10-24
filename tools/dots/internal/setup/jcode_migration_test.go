package setup

import (
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestJcodeRemovesOnlyOurLegacyFeedback(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "personal"}[custom], func(t *testing.T) {
			h, root := jcodeFixture(t)
			command := "'/fixture/node' '" + filepath.Join(h, ".local/share/jcode-integrations/lens/hooks.mjs") + "'"
			if custom {
				command = "my-personal-feedback"
			}
			p := filepath.Join(h, ".jcode/config.toml")
			before, err := toml.Marshal(map[string]any{"hooks": map[string]any{"post_tool_feedback": command, "post_tool_feedback_timeout_ms": int64(30000), "session_end": "keep"}})
			if err != nil {
				t.Fatal(err)
			}
			put(t, p, string(before))
			if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
				t.Fatal(diagnostic)
			}
			var config map[string]any
			if err := toml.Unmarshal([]byte(get(t, p)), &config); err != nil {
				t.Fatal(err)
			}
			hooks := config["hooks"].(map[string]any)
			if _, exists := hooks["post_tool_feedback"]; exists != custom {
				t.Fatal(hooks)
			}
			if hooks["session_end"] != "keep" {
				t.Fatal(hooks)
			}
			if custom && hooks["post_tool_feedback"] != command {
				t.Fatal("personal feedback hook changed")
			}
			if _, exists := hooks["post_tool_feedback_timeout_ms"]; !custom && exists {
				t.Fatal("legacy timeout retained")
			}
		})
	}
}
