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
)

// Real call-site seam: catalog → HTTP client → model → screen. A classic
// histogram whose last finite bucket is 1s cannot distinguish p99=2s from 20s.
// Prometheus returns 1s for either; presenting that as an exact p99 is the bug.
func TestP99BeyondHistogramRangeIsNotPresentedAsExactlyOneSecond(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		query := r.Form.Get("query")
		if strings.HasPrefix(query, "1000 * histogram_quantile(") {
			json.NewEncoder(w).Encode(responseFor(r, "1000"))
			return
		}
		kind := "vector"
		var rows []any
		for i, le := range []string{"0.1", "0.5", "1", "+Inf"} {
			value := []string{"10", "45", "80", "100"}[i]
			at, _ := strconv.ParseFloat(r.Form.Get("time"), 64)
			row := map[string]any{"metric": map[string]string{"le": le}, "value": []any{at, value}}
			if strings.HasSuffix(r.URL.Path, "query_range") {
				kind = "matrix"
				start, _ := strconv.ParseFloat(r.Form.Get("start"), 64)
				end, _ := strconv.ParseFloat(r.Form.Get("end"), 64)
				row = map[string]any{"metric": map[string]string{"le": le}, "values": [][]any{{start, value}, {end, value}}}
			}
			rows = append(rows, row)
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": kind, "result": rows}})
	}))
	defer server.Close()
	p := fixtureProfile()
	p.validate()
	v := newView(p, time.Hour, time.Minute)
	v.chooseMetric("p99")
	c := newClient(server.URL, "")
	defer c.close()
	data, err := c.fetch(context.Background(), v.focused(), v.period, v.step, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	v.apply("p99", data, nil)
	out := v.frame(100, 28, false).Render()
	if !strings.Contains(out, "≥ 1 с") || !strings.Contains(out, "границ") {
		t.Fatalf("p99 beyond the final finite bucket masquerades as exact 1s:\n%s", out)
	}
	if strings.Contains(out, "Сейчас  1 с") {
		t.Fatal("censored quantile was shown as exact")
	}
}
