package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"dots/native/internal/tui"
)

type selection struct {
	PID     int       `json:"pid,omitempty"`
	Version int       `json:"version"`
	Servers string    `json:"servers"`
	Server  string    `json:"server"`
	Checked time.Time `json:"checked"`
	measurement
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func medianLatency(samples []time.Duration) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return samples[len(samples)/2]
}

// Identity is the actual tunnel configuration, not a label or raw URL. A
// subscription can advertise the same route several times under different names.
func (c *controller) serverKey(server object) string {
	cfg, err := c.config(server)
	if err != nil {
		return ""
	}
	return digest(cfg)
}

func (c *controller) candidates() ([]object, error) {
	// Autostart must never prompt or consume Pi's stdin, including first run.
	if _, err := os.Stat(c.path("servers.json")); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(c.path("subscription_url.txt")); err != nil {
			return nil, errors.New("No proxy servers configured. Run pi-proxy --set-sub or --set-vless first.")
		}
	}
	servers, err := c.loadServers()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	unique := make([]object, 0, len(servers))
	for _, server := range servers {
		// Reparse older caches too: fragment text must not leak into transport.
		if raw := value(server, "raw_url", ""); raw != "" {
			if parsed, err := parseVLESS(raw); err == nil {
				server = parsed
			}
		}
		key := c.serverKey(server)
		if key != "" && !seen[key] {
			seen[key] = true
			unique = append(unique, server)
		}
	}
	if len(unique) == 0 {
		return nil, errors.New("No valid proxy servers configured. Run pi-proxy --update.")
	}
	return unique, nil
}

// A previous selection is a hint, not proof of availability. Recheck it live
// even after a long idle period or changes to other subscription entries.
func cachedSelection(cache selection, keys []string, now time.Time) int {
	if cache.Version != 1 || now.Before(cache.Checked) ||
		!allowedCountry(cache.Country) || cache.Latency <= 0 {
		return -1
	}
	for i := range keys {
		if cache.Server == keys[i] {
			return i
		}
	}
	return -1
}

type probeResult struct {
	index int
	measurement
	err error
}

// Stream completed checks so startup need not wait for every route. The caller
// must cancel and drain results when it stops early, reaping all probe children.
// The worker limit bounds xray processes, sockets and upstream load.
func probeRoutes(ctx context.Context, servers []object, workers int, probe func(context.Context, object) (measurement, error)) <-chan probeResult {
	results := make(chan probeResult)
	jobs := make(chan int)
	go func() {
		defer close(jobs)
		for i := range servers {
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				m, err := probe(ctx, servers[i])
				if err == nil && (!allowedCountry(m.Country) || m.Latency <= 0) {
					err = errors.New("exit is RU or measurement is invalid")
				}
				select {
				case results <- probeResult{index: i, measurement: m, err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() { wg.Wait(); close(results) }()
	return results
}

// Exhaustive ranking is reserved for an explicit --auto, not startup.
func benchmark(ctx context.Context, servers []object, workers int, probe func(context.Context, object) (measurement, error)) []probeResult {
	var results []probeResult
	for result := range probeRoutes(ctx, servers, workers, probe) {
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool {
		if (results[i].err == nil) != (results[j].err == nil) {
			return results[i].err == nil
		}
		if results[i].Latency == results[j].Latency {
			return results[i].index < results[j].index
		}
		return results[i].Latency < results[j].Latency
	})
	return results
}

func (c *controller) matchesRunning(server object) bool {
	if c.requireRunning() != nil {
		return false
	}
	data, err := os.ReadFile(c.path("config.json"))
	var cfg object
	return err == nil && json.Unmarshal(data, &cfg) == nil && digest(cfg) == c.serverKey(server)
}

func (c *controller) activate(server object) error {
	if err := c.noForeignListener(); err != nil {
		return err
	}
	if c.matchesRunning(server) {
		return nil
	}
	if err := c.stop(); err != nil {
		return err
	}
	return c.start(server)
}

func (c *controller) autoStart(force bool) error {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	// Serialize selections independently of the short process-control lock.
	unlock, err := c.lockFile("selection.lock", 6*time.Minute)
	if err != nil {
		return err
	}
	defer unlock()
	servers, err := c.candidates()
	if err != nil {
		return err
	}
	keys := make([]string, len(servers))
	for i, server := range servers {
		keys[i] = c.serverKey(server)
	}
	if !force {
		var cached selection
		data, _ := os.ReadFile(c.path("selection.json"))
		if json.Unmarshal(data, &cached) == nil {
			if i := cachedSelection(cached, keys, time.Now()); i >= 0 {
				var m measurement
				running := c.matchesRunning(servers[i])
				if running {
					m, err = c.measureRoute(ctx, c.proxyURL(), 1)
				} else {
					m, err = c.probeServer(ctx, servers[i], 1)
				}
				if err == nil && allowedCountry(m.Country) {
					if !running {
						if err := c.activate(servers[i]); err != nil {
							return err
						}
						// Validate the restarted listener, not just the temporary tunnel.
						m, err = c.measureRoute(ctx, c.proxyURL(), 1)
					}
					if err == nil && c.matchesRunning(servers[i]) {
						return c.saveSelection(keys, i, m)
					}
				}
				if m.Country == "RU" && c.matchesRunning(servers[i]) {
					if err := c.rejectRU(); err != nil {
						return err
					}
				}
			}
		}
	}
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var results <-chan probeResult
	if force {
		fmt.Fprintf(c.out, "Checking %d unique routes: non-RU exit + OpenAI, 4 parallel probes, median of 3 requests...\n", len(servers))
		ranked := benchmark(probeCtx, servers, 4, func(ctx context.Context, server object) (measurement, error) {
			return c.probeServer(ctx, server, 3)
		})
		// Feed the same activation/fallback loop in latency order for --auto.
		ready := make(chan probeResult, len(ranked))
		for _, result := range ranked {
			c.reportProbe(servers[result.index], result)
			ready <- result
		}
		close(ready)
		results = ready
	} else {
		fmt.Fprintf(c.out, "Finding a verified non-RU route to OpenAI: %d routes, 4 parallel checks, stop on first working route...\n", len(servers))
		results = probeRoutes(probeCtx, servers, 4, func(ctx context.Context, server object) (measurement, error) {
			return c.probeServer(ctx, server, 1)
		})
	}
	defer func() {
		cancel()
		for range results {
		}
	}()
	for best := range results {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !force {
			c.reportProbe(servers[best.index], best)
		}
		if best.err != nil {
			continue
		}
		name := tui.Clean(value(servers[best.index], "name", "VLESS server"))
		if err := c.activate(servers[best.index]); err != nil {
			fmt.Fprintln(c.out, "Candidate could not start; trying the next verified route.")
			continue
		}
		// Validate the real listener too, not just the isolated benchmark tunnel.
		live, err := c.measureRoute(ctx, c.proxyURL(), 1)
		if err != nil || !c.matchesRunning(servers[best.index]) {
			if live.Country == "RU" && c.matchesRunning(servers[best.index]) {
				if err := c.rejectRU(); err != nil {
					return err
				}
			}
			fmt.Fprintln(c.out, "Candidate failed its final exit/OpenAI check; trying the next verified route.")
			continue
		}
		if err := c.saveSelection(keys, best.index, live); err != nil {
			return err
		}
		fmt.Fprintf(c.out, "Selected: %s, exit %s, %d ms (verified on the live listener).\n", name, live.Country, live.Latency.Milliseconds())
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.New("No verified non-RU route can reach OpenAI; direct/RU fallback is disabled.")
}

func (c *controller) reportProbe(server object, result probeResult) {
	name := tui.Clean(value(server, "name", "VLESS server"))
	if result.err != nil {
		fmt.Fprintf(c.out, "  SKIP %s: %s\n", name, result.err)
	} else {
		fmt.Fprintf(c.out, "  %s %4d ms  %s\n", result.Country, result.Latency.Milliseconds(), name)
	}
}

func (c *controller) saveSelection(keys []string, index int, m measurement) error {
	pid, key, err := c.liveRoute()
	if err != nil || index < 0 || index >= len(keys) || key != keys[index] || !allowedCountry(m.Country) || m.Latency <= 0 {
		return errors.New("Could not certify the live proxy selection.")
	}
	if err := c.listenerOwned(pid); err != nil {
		return errors.New("Could not certify proxy listener ownership.")
	}
	cached := selection{PID: pid, Version: 1, Servers: digest(keys), Server: keys[index], Checked: time.Now(), measurement: m}
	if err := writeJSON(c.path("selection.json"), cached); err != nil {
		return errors.New("Could not save proxy selection cache.")
	}
	return nil
}
