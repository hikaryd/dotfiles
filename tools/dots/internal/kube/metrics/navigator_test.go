package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"dots/native/internal/testpty"
)

func TestServiceSelectionKeepsMetricAndClearsOldScope(t *testing.T) {
	p := fixtureProfile()
	p.RouteLabel = "handler"
	p.Queries = []metric{
		{ID: "app-pool", Title: "Pool", Section: "db", Query: `pool{namespace="example"}`, ServiceQuery: `pool{namespace="example",service={{service}}}`, Scope: "пул приложения"},
		{ID: "shared-redis", Title: "Redis", Section: "db", Query: `redis{namespace="example"}`, Scope: "общий Redis"},
		{ID: "probe", Title: "Probe", Section: "ingress", Query: "probe_success", NamespaceOnly: true},
	}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	v := newView(p, time.Hour, time.Minute)
	v.chooseMetric("app-pool")
	v.readings["app-pool"] = reading{Data: &snapshot{}}
	service := `api"} or vector(100) #`
	v.setService(service)
	if !v.detail || v.focused().ID != "app-pool" || len(v.readings) != 0 {
		t.Fatal("service change lost location or retained old data")
	}
	if !strings.Contains(v.focused().Query, "service="+strconv.Quote(service)) {
		t.Fatal("service escaped incorrectly")
	}
	if _, err := findMetric(v.items, "probe"); err == nil {
		t.Fatal("unbound namespace probe presented as selected API")
	}
	if !strings.Contains(v.scopeLabel("shared-redis"), "не фильтр сервиса") {
		t.Fatal("shared data looks service-scoped")
	}
	if strings.Contains(p.Queries[0].Query, "service=") || p.Service != "" {
		t.Fatal("selection mutated persistent configuration")
	}
	v.setService("")
	if _, err := findMetric(v.items, "probe"); err != nil {
		t.Fatal("all-services view did not restore namespace metrics")
	}
}

func TestServicePatternsAreQuotedAndTemplatesValidated(t *testing.T) {
	m := metric{ID: "gateway", Title: "Gateway", Query: "up", ServiceQuery: `up{service=~{{service}}}`, ServicePattern: `example\.{{service}}\..+`}
	if err := m.validateService(); err != nil {
		t.Fatal(err)
	}
	bound := m.forService("api.*")
	if !strings.Contains(bound.Query, strconv.Quote(`example\.api\.\*\..+`)) {
		t.Fatal(bound.Query)
	}
	for _, query := range []string{`up{service="{{service}}"}`, `up{service='{{service}}'}`, "up{service=`{{service}}`}", `up{service="prefix{{service}}"}`, `up{service={{other}}}`} {
		m.ServiceQuery = query
		if m.validateService() == nil {
			t.Fatal("unsafe template accepted:", query)
		}
	}
}

func TestPickerSearchIncludesAllMetricsAndTreatsKeysAsText(t *testing.T) {
	v := preview("normal", time.Hour, time.Minute)
	v.openPicker("metrics")
	if len(v.picker.Options) != len(v.items) {
		t.Fatal("catalog is incomplete")
	}
	for _, text := range []string{"q", "u", "e", "u", "e"} {
		if v.pickerKey("text", text) {
			t.Fatal("typing executed navigation")
		}
	}
	if v.picker.Query != "queue" || len(v.picker.matches()) != 2 {
		t.Fatal("search keys were interpreted as commands")
	}
	v.pickerKey("enter", "")
	if !v.detail || !strings.HasPrefix(v.focused().ID, "demo-queue") {
		t.Fatal("search did not open metric")
	}
	v.openPicker("services")
	v.pickerKey("text", "work")
	if len(v.picker.matches()) != 1 {
		t.Fatal("service search failed")
	}
	v.pickerKey("enter", "")
	if v.profile.Service != "worker" || !v.detail {
		t.Fatal("service picker lost the current view")
	}
}

func TestRouteFilterCanBeChangedWithoutRestartOrCommand(t *testing.T) {
	v := preview("normal", time.Hour, time.Minute)
	v.chooseMetric("routep99")
	v.openPicker("route")
	v.pickerKey("text", "[")
	if v.pickerKey("enter", "") || v.profile.RouteFilter != "" || v.picker.Error == "" {
		t.Fatal("invalid regex changed the active view")
	}
	v.pickerKey("ctrl-l", "")
	v.pickerKey("text", "/items.*")
	if !v.pickerKey("enter", "") || v.profile.RouteFilter != "/items.*" || v.focused().ID != "routep99" {
		t.Fatal("route filter did not preserve metric")
	}
	if !strings.Contains(v.focused().Histogram.Query, `handler=~"/items.*"`) || v.readings["routep99"].Data != nil {
		t.Fatal("filter did not change source or retained wrong data")
	}
	if !strings.Contains(v.focused().Histogram.Query, "or on(service,handler,method) (0 *") {
		t.Fatal("idle route can lose its history")
	}
	v.openPicker("route")
	v.pickerKey("ctrl-l", "")
	v.pickerKey("esc", "")
	if v.profile.RouteFilter != "/items.*" {
		t.Fatal("cancel applied an edit")
	}
}

func TestNewPanelsAndCensoredDetailsFitSmallTerminals(t *testing.T) {
	for _, size := range [][2]int{{40, 16}, {80, 24}, {100, 28}, {160, 32}} {
		for _, kind := range []string{"", "services", "metrics", "route"} {
			v := preview("censored", time.Hour, time.Minute)
			v.chooseMetric("routep99")
			if kind != "" {
				v.openPicker(kind)
			}
			out := v.frame(size[0], size[1], false).Render()
			if strings.Contains(out, "More details") {
				t.Fatalf("overflow %v %s:\n%s", size, kind, out)
			}
		}
	}
	if !strings.HasSuffix(inputLine("Фильтр: ", strings.Repeat("x", 100), 40), "▏") {
		t.Fatal("cursor scrolled out of view")
	}
}

func TestSidebarNavigatesSectionsAndPreservesCurrentViewOnReturn(t *testing.T) {
	v := preview("normal", time.Hour, time.Minute)
	v.chooseMetric("routep99")
	out := v.frame(180, 40, false).Render()
	for _, want := range []string{"РАЗДЕЛЫ", "PostgreSQL", "Redis", "Сервис: Все", "/ все метрики"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s:\n%s", want, out)
		}
	}
	v.navFocus = true
	v.navCursor = v.section
	v.confirmNav()
	if !v.detail || v.focused().ID != "routep99" || v.navFocus {
		t.Fatal("returning from section selection lost the current metric")
	}
	v.navCursor, _ = sectionIndex("db")
	v.confirmNav()
	if sections[v.section].ID != "db" || v.detail {
		t.Fatal("section selection did not open its metric list")
	}
}

func TestServiceDiscoveryOnlyReadsNamesInConfiguredNamespace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Method != "POST" || !strings.Contains(r.Form.Get("query"), `namespace="example"`) || strings.Contains(r.Form.Get("query"), `service="pinned"`) {
			t.Error("unsafe discovery scope")
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": []any{map[string]any{"metric": map[string]string{"service": "worker"}, "value": []any{1, "3"}}, map[string]any{"metric": map[string]string{"service": "api"}, "value": []any{1, "5"}}}}})
	}))
	defer server.Close()
	p := fixtureProfile()
	p.Service = "pinned"
	p.validate()
	c := newClient(server.URL, "")
	defer c.close()
	names, err := c.services(context.Background(), p)
	if err != nil || strings.Join(names, ",") != "api,worker" {
		t.Fatalf("names: %v %v", names, err)
	}
}

func TestOneLiveCommandSearchServiceAndMetricWithoutAliases(t *testing.T) {
	seen := make(chan string, 256)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		query := r.Form.Get("query")
		seen <- query
		if strings.HasPrefix(query, "count by (service)") {
			json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": []any{map[string]any{"metric": map[string]string{"service": "example-api"}, "value": []any{1, "1"}}}}})
			return
		}
		json.NewEncoder(w).Encode(responseFor(r, "12.5"))
	}))
	defer server.Close()
	p := fixtureProfile()
	p.URL = server.URL
	terminal := testpty.Start(t, cli(t, "--config", saveConfig(t, p)))
	terminal.Screen("Получены данные: 8 из 8")
	terminal.Send(" ")
	terminal.Screen("ПАУЗА")
	terminal.Send("c")
	terminal.Screen("СЕРВИСЫ")
	terminal.Screen("example-api")
	terminal.Screen("ПАУЗА") // opening discovery must not unfreeze old readings
	terminal.Send("example-api\r")
	terminal.Screen("Сервис: example-api")
	terminal.Send("/")
	terminal.Screen("ВСЕ МЕТРИКИ")
	terminal.Send("q") // searchable text, not quit
	terminal.Screen("Поиск: q")
	terminal.Send("\x7frps\r")
	terminal.Screen("Запросы в секунду (RPS)")
	terminal.Screen("Сейчас  12,5 запр./с")
	terminal.Send("q")
	terminal.Wait(0)
	found := false
	for len(seen) > 0 {
		if strings.Contains(<-seen, `service="example-api"`) {
			found = true
		}
	}
	if !found {
		t.Fatal("service changed visually but not in queries")
	}
}
