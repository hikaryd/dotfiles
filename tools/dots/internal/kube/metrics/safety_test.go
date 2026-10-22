package metrics

import (
	"strings"
	"testing"
)

func TestUnavailableClampsBeforeAggregation(t *testing.T) {
	for _, m := range builtins(profile{Namespace: "app"}) {
		if m.ID == "unavailable" {
			if !strings.HasPrefix(m.Query, "sum(clamp_min(") || !strings.Contains(m.Query, "on(namespace,deployment)") {
				t.Fatal(m.Query)
			}
			return
		}
	}
	t.Fatal("metric missing")
}
