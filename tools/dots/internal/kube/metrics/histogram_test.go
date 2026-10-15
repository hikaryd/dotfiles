package metrics

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testBuckets(values map[float64]float64) buckets {
	b := buckets{}
	for le, value := range values {
		n := value
		b[le] = &n
	}
	return b
}

func TestClassicHistogramBoundsCountsAndInvalidSources(t *testing.T) {
	h := histogramSpec{Quantile: .99, Scale: 1000, CountScale: 1}
	one := testBuckets(map[float64]float64{.1: 0, .5: 1, 1: 2, math.Inf(1): 3})
	p := quantify(1, map[string]buckets{"pod-a": one}, h)
	if p.Value != nil || p.Lower == nil || *p.Lower != 1000 || *p.Samples != 3 || p.Reason != "above-buckets" {
		t.Fatalf("censored: %+v", p)
	}
	if !strings.Contains(sampleNote(&p, .99), "мало") {
		t.Fatal("tiny p99 sample treated as stable")
	}
	if _, err := json.Marshal(p); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		want string
		b    buckets
	}{
		{"no-observations", testBuckets(map[float64]float64{1: 0, math.Inf(1): 0})},
		{"nonmonotonic-buckets", testBuckets(map[float64]float64{1: 10, 2: 9, math.Inf(1): 12})},
		{"incomplete-buckets", testBuckets(map[float64]float64{1: 10, 2: 12})},
	} {
		p := quantify(1, map[string]buckets{"a": tc.b}, h)
		if p.Value != nil || p.Reason != tc.want {
			t.Fatalf("%s: %+v", tc.want, p)
		}
	}
	p = quantify(1, map[string]buckets{"a": one, "b": testBuckets(map[float64]float64{1: 2, 10: 3, math.Inf(1): 3})}, h)
	if p.Reason != "incompatible-buckets" || p.Value != nil {
		t.Fatal("silently merged incompatible bucket layouts")
	}
	h.Quantile = .5
	p = quantify(1, map[string]buckets{"a": testBuckets(map[float64]float64{1: 50, 10: 100, math.Inf(1): 100})}, h)
	if p.Value == nil || *p.Value != 1000 || p.Reason != "estimate" {
		t.Fatal("equality at finite bucket falsely censored")
	}
}

func TestHistogramsMergeObservationsNotPercentiles(t *testing.T) {
	h := histogramSpec{Quantile: .99, Scale: 1, CountScale: 1}
	a := testBuckets(map[float64]float64{1: 99, 10: 100, math.Inf(1): 100})
	b := testBuckets(map[float64]float64{1: 0, 10: 100, math.Inf(1): 100})
	p := quantify(1, map[string]buckets{"a": a, "b": b}, h)
	if p.Value == nil || *p.Value < 9.8 || *p.Value > 9.9 || *p.Samples != 200 {
		t.Fatal("averaged percentiles instead of counts", p)
	}
}

func TestP99BreakdownStaysP99AndSeparateHistogramsAreRespected(t *testing.T) {
	p := fixtureProfile()
	p.DurationMetric = "example_highr"
	p.RouteDurationMetric = "example_routes"
	p.RouteLabel = "handler"
	p.validate()
	v := newView(p, time.Hour, time.Minute)
	v.chooseMetric("p99")
	v.breakdown = true
	if v.focused().Histogram.Quantile != .99 {
		t.Fatal("p99 drilldown silently became p95")
	}
	if !strings.Contains(v.focused().Histogram.Query, "example_highr_bucket") {
		t.Fatal("wrong service histogram")
	}
	v.chooseMetric("routep99")
	if !strings.Contains(v.focused().Histogram.Query, "example_routes_bucket") {
		t.Fatal("global histogram used as per-route data")
	}
}

func TestHistogramFetchBoundsRawWorkAndDoesNotInventCurrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/query") {
			json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": []any{}}})
			return
		}
		json.NewEncoder(w).Encode(responseFor(r, "0"))
	}))
	defer server.Close()
	p := fixtureProfile()
	p.validate()
	v := newView(p, time.Hour, time.Minute)
	v.chooseMetric("p99")
	c := newClient(server.URL, "")
	defer c.close()
	data, err := c.fetch(context.Background(), v.focused(), time.Hour, time.Minute, time.Now())
	if err != nil || len(data.Series) != 1 || data.Series[0].Current != nil || len(data.Series[0].Points) == 0 {
		t.Fatalf("historical value substituted as current: %+v %v", data, err)
	}
}
