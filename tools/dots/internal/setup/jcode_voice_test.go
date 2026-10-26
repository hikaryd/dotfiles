package setup

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func TestJcodeApplyPublishesAgentVoice(t *testing.T) {
	h, root := jcodeFixture(t)
	put(t, filepath.Join(root, "config/jcode/mcp.json"), get(t, filepath.Join("..", "..", "..", "..", "config", "jcode", "mcp.json")))
	dataDir := filepath.Join(h, "data with ' quote")
	t.Setenv("JCODE_INTEGRATIONS_DIR", dataDir)
	if code, _, diagnostic := runCaptured(t, root, "jcode-apply"); code != 0 {
		t.Fatal(code, diagnostic)
	}
	mcp, err := readObject(filepath.Join(h, ".jcode/mcp.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	server, ok := mcp["mcpServers"].(map[string]any)["agent-voice"].(map[string]any)
	if !ok {
		t.Fatal("agent-voice was not published")
	}
	wantPython := filepath.Join(h, "Library/Application Support/VoiceStudio/runtime/project/.venv/bin/python")
	if server["command"] != wantPython {
		t.Fatal("voice MCP must reuse VoiceStudio's installed interpreter", server["command"])
	}
	voicePath := filepath.Join(dataDir, "voice/server.py")
	if !reflect.DeepEqual(server["args"], []any{"-u", voicePath}) {
		t.Fatal("voice MCP must use the published server", server["args"])
	}
	if server["timeout_secs"] != json.Number("600") {
		t.Fatal("cold local model load needs an explicit request timeout")
	}
	env, ok := server["env"].(map[string]any)
	if !ok || env["AGENT_VOICE_CACHE_DIR"] != filepath.Join(dataDir, "voice/audio") || env["HF_HUB_OFFLINE"] != "1" || env["TRANSFORMERS_OFFLINE"] != "1" {
		t.Fatal("voice MCP must keep audio private and never download models implicitly")
	}
	if get(t, voicePath) != "print('fixture voice')\n" {
		t.Fatal("voice server was not published")
	}
}
