package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func responseFor(r *http.Request, value string) any {
	r.ParseForm()
	if strings.HasPrefix(r.Form.Get("query"), "rate(") && strings.Contains(r.Form.Get("query"), "_bucket{") {
		kind := "vector"
		var rows []any
		for i, le := range []string{"0.1", "0.5", "1", "5", "+Inf"} {
			value := []string{"10", "50", "98", "100", "100"}[i]
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
		return map[string]any{"status": "success", "data": map[string]any{"resultType": kind, "result": rows}}
	}
	at, _ := strconv.ParseFloat(r.Form.Get("time"), 64)
	if strings.HasSuffix(r.URL.Path, "query_range") {
		start, _ := strconv.ParseFloat(r.Form.Get("start"), 64)
		end, _ := strconv.ParseFloat(r.Form.Get("end"), 64)
		return map[string]any{"status": "success", "data": map[string]any{"resultType": "matrix", "result": []any{map[string]any{"metric": map[string]string{}, "values": [][]any{{start, value}, {end, value}}}}}}
	}
	return map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": []any{map[string]any{"metric": map[string]string{}, "value": []any{at, value}}}}}
}

func TestInstantAndRangeContractWithPrivatePOST(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer test-secret" || !strings.HasPrefix(r.URL.Path, "/select/tenant/prometheus/api/v1/") {
			t.Errorf("request contract: %s %s", r.Method, r.URL.Path)
		}
		r.ParseForm()
		if r.Form.Get("query") != `sum(rate(requests{namespace="example"}[1m]))` {
			t.Error("query not preserved")
		}
		if strings.HasSuffix(r.URL.Path, "query_range") && r.Form.Get("step") != "60" {
			t.Error("step not seconds")
		}
		json.NewEncoder(w).Encode(responseFor(r, "0"))
	}))
	defer server.Close()
	c := newClient(server.URL+"/select/tenant/prometheus/", "test-secret")
	defer c.close()
	now := time.Unix(1700000000, 0)
	m := metric{ID: "rps", Query: `sum(rate(requests{namespace="example"}[1m]))`, Unit: "req/s"}
	data, err := c.fetch(context.Background(), m, time.Hour, time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || len(data.Series) != 1 || *current(data.Series[0]) != 0 || len(data.Series[0].Points) != 2 || data.Start != float64(now.Unix()-3600) {
		t.Fatalf("snapshot: %+v", data)
	}
}

func TestNoCurrentSampleIsNotHistoricalOrZero(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf", ""} {
		t.Run(value, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "query") {
					if value == "" {
						fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
					} else {
						json.NewEncoder(w).Encode(responseFor(r, value))
					}
				} else {
					json.NewEncoder(w).Encode(responseFor(r, "10"))
				}
			}))
			defer server.Close()
			c := newClient(server.URL, "")
			defer c.close()
			data, err := c.fetch(context.Background(), metric{ID: "test", Query: "up"}, time.Hour, time.Minute, time.Now())
			if err != nil || len(data.Series) != 1 || current(data.Series[0]) != nil {
				t.Fatalf("nonfinite current: %+v %v", data, err)
			}
			if data.Series[0].Points[0].Value == nil {
				t.Fatal("history lost")
			}
			if _, err := json.Marshal(data); err != nil {
				t.Fatal("nonfinite JSON not normalized")
			}
		})
	}
}

func TestAPIRejectsErrorsPartialMalformedAndOversizeWithoutLeaks(t *testing.T) {
	cases := []struct {
		status int
		body   string
	}{
		{401, "PRIVATE_TOKEN"},
		{200, `{"status":"error","error":"PRIVATE_TOKEN"}`},
		{200, `{"status":"success","isPartial":true,"data":{"resultType":"vector","result":[]}}`},
		{200, `{"status":"success","warnings":["PRIVATE_TOKEN"],"data":{"resultType":"vector","result":[]}}`},
		{200, `{"status":"success","data":{"resultType":"scalar","result":[]}}`},
		{200, `{"status":"success","data":{"resultType":"vector","result":` + strings.Repeat(`{"metric":{},"value":[1,"1"]},`, 128) + `{"metric":{},"value":[1,"1"]}]}}`},
		{200, strings.Repeat("x", maxResponse+1)},
	}
	for i, tc := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
		c := newClient(server.URL, "test-secret")
		_, err := c.query(context.Background(), "/api/v1/query", url.Values{"query": {"up"}}, "vector")
		if err == nil || strings.Contains(err.Error(), "PRIVATE") || strings.Contains(err.Error(), server.URL) {
			t.Fatalf("case %d diagnostic: %v", i, err)
		}
		c.close()
		server.Close()
	}
}

func TestClientDoesNotRedirectOrLeakAuthorization(t *testing.T) {
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer server.Close()
	c := newClient(server.URL, "test-secret")
	defer c.close()
	_, err := c.query(context.Background(), "/api/v1/query", url.Values{}, "vector")
	if err == nil || leaked.Load() {
		t.Fatalf("redirect: %v leaked=%v", err, leaked.Load())
	}
}

func TestCancellationAndBoundedBatch(t *testing.T) {
	var active, peak atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(20 * time.Millisecond):
		}
		json.NewEncoder(w).Encode(responseFor(r, "3"))
	}))
	defer server.Close()
	c := newClient(server.URL, "")
	defer c.close()
	items := make([]metric, 10)
	for i := range items {
		items[i] = metric{ID: fmt.Sprint(i), Query: "up"}
	}
	var count atomic.Int32
	fetchBatch(context.Background(), c, items, time.Hour, time.Minute, func(_ string, _ snapshot, err error) {
		if err != nil {
			t.Error(err)
		}
		count.Add(1)
	})
	if count.Load() != 10 || peak.Load() > 4 {
		t.Fatalf("batch count=%d concurrency=%d", count.Load(), peak.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, err := c.fetch(ctx, items[0], time.Hour, time.Minute, start)
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("cancel: %v", err)
	}
}

func TestMalformedSamples(t *testing.T) {
	for _, sample := range []string{`[]`, `[1]`, `[1,4]`, `[1,"nope"]`, `["wrong","1"]`} {
		var raw []json.RawMessage
		json.Unmarshal([]byte(sample), &raw)
		if _, err := decodePoint(raw); err == nil {
			t.Fatalf("accepted %s", sample)
		}
	}
}
