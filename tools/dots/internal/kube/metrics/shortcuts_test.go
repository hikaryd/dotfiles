package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestShortcutUsesOnlyPrivateProfileMappingAndKeepsArguments(t *testing.T) {
	server := func(value string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(responseFor(r, value)) }))
	}
	all, api := server("12.5"), server("3.15")
	defer all.Close()
	defer api.Close()
	p, q := fixtureProfile(), fixtureProfile()
	p.Name, p.URL = "all", all.URL
	q.Name, q.URL = "only-api", api.URL
	path := saveConfig(t, p, q)
	data, _ := json.Marshal(inventory{Profiles: []profile{p, q}, Shortcuts: map[string]string{"km": "all", "kma": "only-api"}})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		value float64
	}{{"km", 12.5}, {"kma", 3.15}} {
		cmd := cli(t, "rps", "--config", path, "--range", "15m", "--json")
		cmd.Args[0] = tc.name
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %s %v", tc.name, out, err)
		}
		var got snapshot
		if json.Unmarshal(out, &got) != nil || got.Metric != "rps" || *current(got.Series[0]) != tc.value || got.End-got.Start != 900 {
			t.Fatalf("shortcut target/argv lost: %s", out)
		}
	}
}

func TestAPIShortcutDoesNotGuessProfileOrConnectWhenUnconfigured(t *testing.T) {
	p := fixtureProfile()
	path := saveConfig(t, p)
	cmd := cli(t, "--config", path)
	cmd.Args[0] = "kma"
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "shortcuts.kma") || strings.Contains(string(out), p.URL) {
		t.Fatalf("missing shortcut: %s %v", out, err)
	}
	for _, mapping := range []map[string]string{{"kma": "missing"}, {"arbitrary": "test"}} {
		data, _ := json.Marshal(inventory{Profiles: []profile{p}, Shortcuts: mapping})
		os.WriteFile(path, data, 0600)
		if _, err := loadConfig(path); err == nil {
			t.Fatal("invalid shortcut map accepted")
		}
	}
}

func TestShortMetricSyntaxHasNoConfigSideEffects(t *testing.T) {
	o, action, err := parse([]string{"rps", "--range", "15m", "--preview"})
	if err != nil || action != "watch" || o.metric != "rps" || !o.preview {
		t.Fatalf("short syntax: %+v %s %v", o, action, err)
	}
	for _, name := range []string{"km", "kma"} {
		cmd := cli(t, "--help", "--config", "/missing")
		cmd.Args[0] = name
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("shortcut help touched config: %s %v", out, err)
		}
	}
}
