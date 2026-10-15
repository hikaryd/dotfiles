package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dots/native/internal/testpty"
)

func TestSectionShortcutsPreviewAndOfflineList(t *testing.T) {
	for _, s := range sections {
		out, err := cli(t, s.ID, "--preview", "--config", "/missing/private/file", "--width", "80", "--height", "24").CombinedOutput()
		if err != nil || !strings.Contains(string(out), s.Title) {
			t.Fatalf("%s: %s %v", s.ID, out, err)
		}
	}
	out, err := cli(t, "--sections", "--config", "/missing/private/file").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "outbound") || !strings.Contains(string(out), "runtime") {
		t.Fatalf("%s %v", out, err)
	}
	for _, args := range [][]string{{"--preview", "--section", "typo"}, {"--preview", "--section", "db", "--metric", "rps"}, {"--preview", "--route", "["}} {
		if out, err := cli(t, args...).CombinedOutput(); err == nil {
			t.Fatalf("accepted invalid selection: %s", out)
		}
	}
}

func TestEmptySectionDoesNotConnectEvenInLiveMode(t *testing.T) {
	p, dir := fakeKubectl(t)
	path := saveConfig(t, p)
	for _, args := range [][]string{{"runtime", "--config", path, "--once"}, {"--section", "outbound", "--config", path, "--json"}} {
		if out, err := cli(t, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s %v", out, err)
		}
	}
	terminal := testpty.Start(t, cli(t, "runtime", "--config", path))
	terminal.Screen("Нет настроенных показателей")
	if _, err := os.Stat(filepath.Join(dir, "pid")); !os.IsNotExist(err) {
		t.Fatal("empty section opened a port-forward")
	}
	terminal.Send("s")
	terminal.Screen("РАЗДЕЛЫ")
	terminal.Send("\x1b[200~q123s\x1b[201~")
	terminal.Send("\x1b")
	terminal.Send("1")
	terminal.Screen("Получены данные: 8 из 8")
	terminal.Send("q")
	terminal.Wait(0)
}

func TestSectionMenuSelectsGroupedQueryAndPreservesMetricIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(responseFor(r, "12.5")) }))
	defer server.Close()
	p := fixtureProfile()
	p.URL = server.URL
	p.Queries = []metric{{ID: "runtime-heap", Title: "Heap приложения", Query: "example_heap", Section: "runtime", Unit: "MiB", Scope: "только example-api"}}
	terminal := testpty.Start(t, cli(t, "--config", saveConfig(t, p)))
	terminal.Screen("Получены данные: 8 из 8")
	terminal.Send("s")
	terminal.Screen("РАЗДЕЛЫ")
	terminal.Send("\x1b[A") // overview wraps to runtime
	terminal.Screen("› Runtime")
	terminal.Send("\r")
	terminal.Screen("Heap приложения")
	terminal.Send("\r")
	terminal.Screen("Сейчас  12,5 МиБ")
	terminal.Screen("только example-api")
	terminal.Send("[")
	terminal.Screen("Вход и доступность")
	terminal.Screen("Нет настроенных показателей")
	terminal.Send("q")
	terminal.Wait(0)
}

func TestRouteRequestsPinTheSameAbsoluteRankingTime(t *testing.T) {
	queries := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		queries <- r.Form.Get("query")
		json.NewEncoder(w).Encode(responseFor(r, "1"))
	}))
	defer server.Close()
	p := fixtureProfile()
	p.RouteLabel = "handler"
	p.validate()
	c := newClient(server.URL, "")
	defer c.close()
	_, err := c.fetch(context.Background(), routeMetrics(p)[0], time.Hour, time.Minute, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	a, b := <-queries, <-queries
	if a != b || !strings.Contains(a, "@ 1700000000") || strings.Contains(a, "@ end()") {
		t.Fatalf("ranking timestamps differ: %s / %s", a, b)
	}
}

func TestSectionSelectionDoesNotHijackExistingCustomMetricID(t *testing.T) {
	p := fixtureProfile()
	p.Queries = []metric{{ID: "db", Title: "Legacy db", Query: "up"}}
	p.validate()
	v := newView(p, time.Hour, time.Minute)
	if err := selectView(&v, options{metric: "db", positional: true}); err != nil || !v.detail || v.focused().ID != "db" {
		t.Fatal("legacy metric lost precedence")
	}
	if err := selectView(&v, options{section: "db"}); err != nil || v.detail {
		t.Fatal("explicit section did not select section")
	}
}
