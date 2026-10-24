package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxResponse = 8 * 1024 * 1024

type point struct {
	At      float64  `json:"timestamp"`
	Value   *float64 `json:"value"` // NaN/Inf and censored estimates are null, never zero.
	Lower   *float64 `json:"lower_bound,omitempty"`
	Upper   *float64 `json:"upper_bound,omitempty"`
	Samples *float64 `json:"observations,omitempty"`
	Reason  string   `json:"quality,omitempty"`
}

type series struct {
	Labels  map[string]string `json:"labels"`
	Current *point            `json:"current,omitempty"`
	Points  []point           `json:"points"`
}

type snapshot struct {
	Metric    string    `json:"metric"`
	Unit      string    `json:"unit"`
	FetchedAt time.Time `json:"fetched_at"`
	Start     float64   `json:"start"`
	End       float64   `json:"end"`
	Step      float64   `json:"step_seconds"`
	Quantile  float64   `json:"quantile,omitempty"`
	Series    []series  `json:"series"`
}

type apiResult struct {
	Metric map[string]string   `json:"metric"`
	Value  []json.RawMessage   `json:"value"`
	Values [][]json.RawMessage `json:"values"`
}

type apiResponse struct {
	Status   string            `json:"status"`
	Partial  bool              `json:"isPartial"`
	Warnings []json.RawMessage `json:"warnings"`
	Data     struct {
		ResultType string      `json:"resultType"`
		Result     []apiResult `json:"result"`
	} `json:"data"`
}

type client struct {
	base  string
	token string
	http  *http.Client
}

func newClient(base, token string) *client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Private loopback forwards must not travel through shell HTTP proxies.
	transport.Proxy = nil
	return &client{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{
		Transport:     transport,
		Timeout:       12 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (c *client) close() { c.http.CloseIdleConnections() }

func (c *client) query(ctx context.Context, path string, args url.Values, kind string) ([]apiResult, error) {
	return c.queryLimit(ctx, path, args, kind, 128)
}

func (c *client) queryLimit(ctx context.Context, path string, args url.Values, kind string, limit int) ([]apiResult, error) {
	// POST keeps PromQL/namespace/service out of URL access logs. Never expose
	// transport errors, server bodies or request URLs in diagnostics.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, strings.NewReader(args.Encode()))
	if err != nil {
		return nil, errors.New("cannot construct metrics request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("metrics connection failed or timed out; check the private connection locally")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics API returned HTTP %d (body hidden)", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return nil, errors.New("metrics response unreadable or exceeds 8 MiB")
	}
	var decoded apiResponse
	if json.Unmarshal(data, &decoded) != nil || decoded.Status != "success" || decoded.Data.ResultType != kind {
		return nil, errors.New("metrics API rejected the query or returned an invalid response (details hidden)")
	}
	if decoded.Partial || len(decoded.Warnings) > 0 {
		return nil, errors.New("metrics API reported partial data or warnings (details hidden)")
	}
	if len(decoded.Data.Result) > limit {
		return nil, fmt.Errorf("query returned over %d series; narrow the private query", limit)
	}
	return decoded.Data.Result, nil
}

func decodePoint(raw []json.RawMessage) (point, error) {
	var p point
	var value string
	if len(raw) != 2 || json.Unmarshal(raw[0], &p.At) != nil || math.IsNaN(p.At) || math.IsInf(p.At, 0) || json.Unmarshal(raw[1], &value) != nil {
		return p, errors.New("invalid metrics sample")
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return p, errors.New("invalid metrics sample value")
	}
	if !math.IsNaN(n) && !math.IsInf(n, 0) {
		p.Value = &n
	}
	return p, nil
}

func labelKey(labels map[string]string) string {
	// encoding/json orders map keys and avoids delimiter collisions.
	data, _ := json.Marshal(labels)
	return string(data)
}

func (c *client) fetch(ctx context.Context, m metric, period, step time.Duration, now time.Time) (snapshot, error) {
	if m.Histogram != nil {
		return c.fetchHistogram(ctx, m, period, step, now)
	}
	end := float64(now.Unix())
	start := end - period.Seconds()
	result := snapshot{Metric: m.ID, Unit: m.Unit, FetchedAt: now, Start: start, End: end, Step: step.Seconds(), Series: []series{}}
	query := m.Query
	if m.queryAt != nil {
		query = m.queryAt(strconv.FormatFloat(end, 'f', -1, 64))
	}
	args := url.Values{"query": {query}, "start": {strconv.FormatFloat(start, 'f', -1, 64)}, "end": {strconv.FormatFloat(end, 'f', -1, 64)}, "step": {strconv.FormatFloat(step.Seconds(), 'f', -1, 64)}}
	var history, current []apiResult
	var historyErr, currentErr error
	// Two independent requests, both covered by the caller's cancellation.
	done := make(chan struct{})
	go func() {
		defer close(done)
		current, currentErr = c.query(ctx, "/api/v1/query", url.Values{"query": {query}, "time": {args.Get("end")}}, "vector")
	}()
	history, historyErr = c.query(ctx, "/api/v1/query_range", args, "matrix")
	<-done
	if historyErr != nil {
		return result, historyErr
	}
	if currentErr != nil {
		return result, currentErr
	}
	byLabel := map[string]*series{}
	totalSamples := 0
	for _, item := range history {
		totalSamples += len(item.Values)
		if totalSamples > 20000 {
			return result, errors.New("over 20000 samples; narrow the query or increase --step")
		}
		if len(item.Values) > 1500 {
			return result, errors.New("too many samples; increase --step")
		}
		key := labelKey(item.Metric)
		if byLabel[key] != nil {
			return result, errors.New("duplicate metric series")
		}
		s := &series{Labels: item.Metric, Points: []point{}}
		for _, raw := range item.Values {
			p, err := decodePoint(raw)
			if err != nil {
				return result, err
			}
			if len(s.Points) > 0 && p.At <= s.Points[len(s.Points)-1].At {
				return result, errors.New("metrics samples are not ordered")
			}
			s.Points = append(s.Points, p)
		}
		byLabel[key] = s
	}
	seenCurrent := map[string]bool{}
	for _, item := range current {
		p, err := decodePoint(item.Value)
		if err != nil {
			return result, err
		}
		key := labelKey(item.Metric)
		if seenCurrent[key] {
			return result, errors.New("duplicate current series")
		}
		seenCurrent[key] = true
		if byLabel[key] == nil {
			byLabel[key] = &series{Labels: item.Metric, Points: []point{}}
		}
		byLabel[key].Current = &p
	}
	if len(byLabel) > 128 {
		return result, errors.New("query returned over 128 series; narrow the private query")
	}
	for _, s := range byLabel {
		result.Series = append(result.Series, *s)
	}
	sort.Slice(result.Series, func(i, j int) bool {
		return bytes.Compare([]byte(labelKey(result.Series[i].Labels)), []byte(labelKey(result.Series[j].Labels))) < 0
	})
	result.FetchedAt = time.Now()
	return result, nil
}
