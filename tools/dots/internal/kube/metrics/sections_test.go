package metrics

import (
	"strings"
	"testing"
	"time"

	"dots/native/internal/tui"
)

func TestGroupedQueriesAreScopedAndOnlyVisibleSectionIsPolled(t *testing.T) {
	p := fixtureProfile()
	p.RouteLabel = "handler"
	p.Queries = []metric{
		{ID: "db-pool", Title: "Занятые соединения", Query: `pool_active{namespace="example"}`, Section: "db", Scope: "пул приложения"},
		{ID: "queue-depth", Title: "Ожидающие задачи", Query: `queue_depth{namespace="example"}`, Section: "queues"},
		{ID: "custom", Title: "Своя метрика", Query: "up"},
	}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	v := newView(p, time.Hour, time.Minute)
	for section, id := range map[string]string{"db": "db-pool", "queues": "queue-depth", "custom": "custom"} {
		if err := v.chooseSection(section); err != nil {
			t.Fatal(err)
		}
		plan := v.plan()
		if len(plan) != 1 || plan[0].ID != id {
			t.Fatalf("%s plan: %+v", section, plan)
		}
	}
	if err := v.chooseMetric("db-pool"); err != nil {
		t.Fatal(err)
	}
	if !v.detail || v.scopeLabel("db-pool") != "пул приложения" {
		t.Fatal("group detail lost scope")
	}
	if err := v.chooseSection("runtime"); err != nil {
		t.Fatal(err)
	}
	if len(v.plan()) != 0 {
		t.Fatal("unconfigured runtime polled something unrelated")
	}
	out := v.frame(100, 28, false).Render()
	if !strings.Contains(out, "Нет настроенных показателей") || !strings.Contains(out, "runtime") {
		t.Fatal(out)
	}
}

func TestRouteQueriesKeepRouteMethodAndServiceIdentity(t *testing.T) {
	p := fixtureProfile()
	p.Service = "example-api"
	p.RouteLabel = "handler"
	p.RouteFilter = `/items/(.*|[1m])`
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	v := newView(p, time.Hour, time.Minute)
	if err := v.chooseSection("routes"); err != nil {
		t.Fatal(err)
	}
	if len(v.rows()) < 4 {
		t.Fatal("route metrics missing")
	}
	for _, m := range v.rows() {
		for _, part := range []string{`namespace="example"`, `service="example-api"`, `handler!=""`, "service,handler,method"} {
			if !strings.Contains(m.Query, part) {
				t.Fatalf("%s missing %s: %s", m.ID, part, m.Query)
			}
		}
		if strings.Contains(m.Query, "or vector(0)") {
			t.Fatal("fabricates zeros")
		}
		if !strings.Contains(m.Query, "topk(10,") || !strings.Contains(m.Query, "@ end()") || !strings.Contains(m.Title, "Топ-10") {
			t.Fatal("top ten must be explicit and stable over history")
		}
	}
	for _, m := range v.rows() {
		pinned := m.queryAt("1700000000")
		if !strings.Contains(pinned, "@ 1700000000") || strings.Contains(pinned, "@ end()") || !strings.Contains(pinned, `handler=~"/items/(.*|[1m])"`) {
			t.Fatal("cohort timestamp or literal route filter changed:", pinned)
		}
	}
	i, _ := findMetric(v.items, "routep95")
	if !strings.Contains(v.items[i].Query, "histogram_quantile(0.95") || !strings.Contains(v.items[i].Query, "le,service,handler,method") {
		t.Fatal(v.items[i])
	}
}

func TestGroupedConfigurationValidationAndLegacyCompatibility(t *testing.T) {
	for _, change := range []func(*profile){
		func(p *profile) { p.RouteLabel = `handler"}` },
		func(p *profile) { p.RouteLabel = "service" },
		func(p *profile) { p.RouteLabel = "method" },
		func(p *profile) { p.RouteFilter = "[" },
		func(p *profile) { p.Queries = []metric{{ID: "pool", Title: "Pool", Query: "up", Section: "typo"}} },
		func(p *profile) { p.SectionNotes = map[string]string{"typo": "unknown"} },
		func(p *profile) { p.SectionNotes = map[string]string{"runtime": strings.Repeat("x", 2049)} },
		func(p *profile) {
			p.Queries = []metric{{ID: "db", Title: "Too long scope", Query: "up", Scope: strings.Repeat("x", 301)}}
		},
	} {
		p := fixtureProfile()
		change(&p)
		if _, err := loadConfig(saveConfig(t, p)); err == nil {
			t.Fatal("invalid grouped config accepted")
		}
	}
	p := fixtureProfile()
	p.CustomOnly = true
	p.Queries = []metric{{ID: "custom", Title: "Legacy", Query: "up"}}
	cfg, err := loadConfig(saveConfig(t, p))
	if err != nil {
		t.Fatal(err)
	}
	v := newView(cfg.Profiles[0], time.Hour, time.Minute)
	if len(v.rows()) != 1 || v.rows()[0].ID != "custom" {
		t.Fatal("legacy queries moved")
	}
	p.Queries[0].Section = "runtime"
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	v = newView(p, time.Hour, time.Minute)
	if len(v.rows()) != 1 {
		t.Fatal("grouped custom-only profile starts empty")
	}
}

func TestAllSectionsAndPickerFitNarrowTerminals(t *testing.T) {
	for _, size := range [][2]int{{280, 50}, {100, 28}, {80, 24}, {40, 16}} {
		v := preview("normal", time.Hour, time.Minute)
		for i, section := range sections {
			if err := v.chooseSection(section.ID); err != nil {
				t.Fatal(err)
			}
			for _, menu := range []bool{false, true} {
				v.showSections = menu
				v.sectionCursor = i
				out := v.frame(size[0], size[1], false).Render()
				if strings.Contains(out, "More details are available") {
					t.Fatalf("hidden content %s %v: %s", section.ID, size, out)
				}
				if !strings.Contains(out, section.Title) || !strings.Contains(out, "q выход") {
					t.Fatalf("lost section/exit %s %v: %s", section.ID, size, out)
				}
				for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
					if tui.Width(line) > size[0] {
						t.Fatal("width overflow")
					}
				}
			}
		}
	}
}

func TestUnavailableSectionNotesAreNotPresentedAsMeasuredZero(t *testing.T) {
	p := fixtureProfile()
	p.SectionNotes = map[string]string{"outbound": "В проверенной области клиентских метрик нет; добавьте HTTP client instrumentation."}
	v := newView(p, time.Hour, time.Minute)
	v.chooseSection("outbound")
	out := v.frame(100, 28, false).Render()
	if !strings.Contains(out, "HTTP client instrumentation") || strings.Contains(out, "Получены данные: 0") {
		t.Fatal(out)
	}
	if len(v.plan()) != 0 {
		t.Fatal("empty section generates a request")
	}
}
