package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestJcodeApplyRemovesLegacyAudio(t *testing.T) {
	for _, key := range []string{"servers", "mcpServers"} {
		t.Run(key, func(t *testing.T) {
			h, root := jcodeFixture(t)
			tracked := filepath.Join("..", "..", "..", "..", "config", "jcode")
			put(t, filepath.Join(root, "config/jcode/config.toml"), get(t, filepath.Join(tracked, "config.toml")))
			put(t, filepath.Join(root, "config/jcode/mcp.json"), get(t, filepath.Join(tracked, "mcp.json")))
			configPath := filepath.Join(h, ".jcode/config.toml")
			mcpPath := filepath.Join(h, ".jcode/mcp.json")
			put(t, configPath, "[dictation]\ncommand = 'personal'\n[keybindings]\nvoice_input = 'ctrl+space'\n[notifications]\nturn_complete_sound = 'Glass'\n[providers.private]\napi_key_env = 'PRIVATE_KEY'\n[hooks]\nsession_end = 'personal-hook'\n")
			put(t, mcpPath, `{"`+key+`":{"agent-voice":{"command":"old"},"personal":{"command":"keep"}},"personalOption":true}`)
			voice := filepath.Join(h, ".local/share/jcode-integrations/voice/server.py")
			cache := filepath.Join(h, ".local/share/jcode-integrations/voice/audio/old.wav")
			put(t, voice, "legacy server")
			put(t, cache, "legacy audio")
			for i := 0; i < 2; i++ {
				if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
					t.Fatal(code, diagnostic)
				}
				var config map[string]any
				if err := toml.Unmarshal([]byte(get(t, configPath)), &config); err != nil {
					t.Fatal(err)
				}
				if _, exists := config["dictation"]; exists {
					t.Fatal("dictation retained")
				}
				if config["keybindings"].(map[string]any)["voice_input"] != "" || config["notifications"].(map[string]any)["turn_complete_sound"] != "" {
					t.Fatal("audio not disabled")
				}
				if config["providers"].(map[string]any)["private"].(map[string]any)["api_key_env"] != "PRIVATE_KEY" || config["hooks"].(map[string]any)["session_end"] != "personal-hook" {
					t.Fatal("personal settings changed")
				}
				mcp, err := readObject(mcpPath, false)
				if err != nil {
					t.Fatal(err)
				}
				servers := mcp[key].(map[string]any)
				if _, exists := servers["agent-voice"]; exists {
					t.Fatal("legacy server retained")
				}
				if servers["personal"].(map[string]any)["command"] != "keep" || mcp["personalOption"] != true || servers["serena"] == nil {
					t.Fatal("other MCP settings changed")
				}
				if get(t, voice) != "legacy server" || get(t, cache) != "legacy audio" {
					t.Fatal("legacy files changed")
				}
			}
		})
	}
}

func TestJcodeApplyDoesNotCreateVoiceServer(t *testing.T) {
	h, root := jcodeFixture(t)
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
		t.Fatal(code, diagnostic)
	}
	if _, err := os.Lstat(filepath.Join(h, ".local/share/jcode-integrations/voice/server.py")); !os.IsNotExist(err) {
		t.Fatalf("voice server unexpectedly exists: %v", err)
	}
}
