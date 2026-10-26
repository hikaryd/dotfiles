package beatapi

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func openRouterConfig(t *testing.T, key string) Config {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"api_key": "test-secret", "port": 6446,
		"proxy_url": "http://127.0.0.1:10810", "openrouter_api_key": key,
	})
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestOpenRouterConfigRoundTripsBothKeys(t *testing.T) {
	cfg := openRouterConfig(t, strings.Repeat("<", 8192))
	cfg.APIKey = strings.Repeat(">", 8192)
	p := configPath(t)
	if err := saveConfig(p, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	var fields map[string]any
	json.Unmarshal(b, &fields)
	if fields["openrouter_api_key"] != strings.Repeat("<", 8192) || got.APIKey != cfg.APIKey {
		t.Fatal("saving OpenRouter configuration lost a provider key")
	}
	st, err := os.Stat(p)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("OpenRouter key is not private")
	}
}

func TestOpenRouterConfigRejectsUnsafeKeys(t *testing.T) {
	for _, key := range []string{"public", "a b", "a\nb", "a\x00b", strings.Repeat("x", 8193)} {
		if err := saveConfig(configPath(t), openRouterConfig(t, key)); err == nil {
			t.Fatal("unsafe OpenRouter key accepted")
		} else if key != "public" && strings.Contains(err.Error(), key) {
			t.Fatal("configuration error exposed an OpenRouter key")
		}
	}
}

func TestOpenRouterCLIKeyPreservesBeatAPIAndJevgrep(t *testing.T) {
	c := testCLI(t)
	if err := saveConfig(c.config, testConfig()); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"openrouter-secret", "rotated-openrouter-secret"} {
		if code := Run([]string{"openrouter-key", "--stdin"}, strings.NewReader(key+"\n"), c.out, c.errOut); code != 0 {
			t.Fatalf("OpenRouter key configuration failed: %v", c.errOut)
		}
		b, err := os.ReadFile(c.config)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		json.Unmarshal(b, &fields)
		if fields["openrouter_api_key"] != key || fields["api_key"] != "test-secret" || fields["port"] != float64(6446) {
			t.Fatal("OpenRouter key rotation damaged provider configuration")
		}
		if strings.Contains(fmt.Sprint(c.out, c.errOut), key) {
			t.Fatal("OpenRouter key leaked to CLI output")
		}
	}
	if code := Run([]string{"key", "--stdin"}, strings.NewReader("rotated-beat-secret\n"), c.out, c.errOut); code != 0 {
		t.Fatal("BeatAPI key rotation failed after configuring OpenRouter")
	}
	b, _ := os.ReadFile(c.config)
	var fields map[string]any
	json.Unmarshal(b, &fields)
	if fields["openrouter_api_key"] != "rotated-openrouter-secret" || fields["api_key"] != "rotated-beat-secret" {
		t.Fatal("BeatAPI key rotation lost OpenRouter configuration")
	}
	if _, err := os.Stat(c.backupPath()); !os.IsNotExist(err) {
		t.Fatal("OpenRouter key command changed Jevgrep")
	}
}

func TestOpenRouterCLIRequiresExplicitStdin(t *testing.T) {
	c := testCLI(t)
	if err := saveConfig(c.config, testConfig()); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"openrouter-key"}, strings.NewReader("not-to-be-read\n"), c.out, c.errOut); code == 0 || !strings.Contains(fmt.Sprint(c.errOut), "--stdin") {
		t.Fatal("OpenRouter key was accepted without hidden input or explicit stdin")
	}
}

func TestOpenRouterStatusShowsConfiguredRouteWithoutSecrets(t *testing.T) {
	for _, key := range []string{"", "openrouter-secret"} {
		t.Run(fmt.Sprint(key != ""), func(t *testing.T) {
			c := testCLI(t)
			fakeCommand(t, "launchctl", "exit 113\n")
			if err := saveConfig(c.config, openRouterConfig(t, key)); err != nil {
				t.Fatal(err)
			}
			c.status()
			out := fmt.Sprint(c.out, c.errOut)
			if !strings.Contains(out, fmt.Sprintf("OpenRouter configured: %t", key != "")) {
				t.Fatal("status hides the OpenRouter configuration state")
			}
			if key != "" && (!strings.Contains(out, "liquid/d1-ultra") || !strings.Contains(out, "fallback")) {
				t.Fatal("status does not identify the primary and fallback")
			}
			for _, secret := range []string{"openrouter-secret", "test-secret"} {
				if strings.Contains(out, secret) {
					t.Fatal("status leaked a provider key")
				}
			}
		})
	}
}
