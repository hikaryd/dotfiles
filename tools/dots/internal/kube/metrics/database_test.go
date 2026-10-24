package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestPostgresAndRedisAreSeparateAndMissingCollectionDoesNotPoll(t *testing.T) {
	p := fixtureProfile()
	p.validate()
	p.Queries = []metric{{ID: "redis-clients", Title: "Клиенты", Unit: "clients", Section: "redis", Query: "example_redis_clients"}}
	v := newView(p, time.Hour, time.Minute)
	v.chooseSection("postgres")
	if len(v.plan()) != 0 {
		t.Fatal("PostgreSQL section substituted Redis or unrelated collectors")
	}
	out := v.frame(100, 28, false).Render()
	for _, want := range []string{"PostgreSQL", "Нет настроенных показателей", "postgres_exporter"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s:\n%s", want, out)
		}
	}
	v.chooseSection("redis")
	if len(v.plan()) != 1 || v.plan()[0].ID != "redis-clients" {
		t.Fatal("Redis source disappeared")
	}
}
