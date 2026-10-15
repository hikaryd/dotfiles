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

const selectionTTL = 30 * time.Minute

type selection struct {
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

func cachedSelection(cache selection, servers []object, keys []string, now time.Time) int {
	age := now.Sub(cache.Checked)
	if cache.Version != 1 || cache.Servers != digest(keys) || age < 0 || age >= selectionTTL ||
		!allowedCountry(cache.Country) || cache.Latency <= 0 {
		return -1
	}
	for i := range servers {
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

// The worker limit bounds xray processes, open sockets and load on the upstream.
// Results are reported by the caller, never concurrently into CLI writers.
func benchmark(ctx context.Context, servers []object, workers int, probe func(context.Context, object) (measurement, error)) []probeResult {
	results := make([]probeResult, len(servers))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i].index = i
				if ctx.Err() != nil {
					results[i].err = ctx.Err()
					continue
				}
				results[i].measurement, results[i].err = probe(ctx, servers[i])
				if results[i].err == nil && (!allowedCountry(results[i].Country) || results[i].Latency <= 0) {
					results[i].err = errors.New("exit is RU or measurement is invalid")
				}
			}
		}()
	}
	for i := range servers {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	sort.SliceStable(results, func(i, j int) bool {
		if (results[i].err == nil) != (results[j].err == nil) {
			return results[i].err == nil
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
	// Serialize full selections independently of the short process-control lock.
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
			if i := cachedSelection(cached, servers, keys, time.Now()); i >= 0 {
				var m measurement
				running := c.matchesRunning(servers[i])
				if running {
					m, err = measureRoute(ctx, c.proxyURL(), countryURL, openAIURL, 1)
				} else {
					m, err = c.probeServer(ctx, servers[i])
				}
				if err == nil && allowedCountry(m.Country) {
					if running {
						return nil
					}
					if err := c.activate(servers[i]); err != nil {
						return err
					}
					// Recheck a restarted listener, not just the temporary tunnel.
					if !c.matchesRunning(servers[i]) {
						return errors.New("Selected proxy is not running; Pi was not started.")
					}
					_, err = measureRoute(ctx, c.proxyURL(), countryURL, openAIURL, 1)
					if err == nil {
						return nil
					}
				}
			}
		}
	}
	fmt.Fprintf(c.out, "Checking %d unique routes: non-RU exit + OpenAI, 4 parallel probes, median of 3 requests...\n", len(servers))
	results := benchmark(ctx, servers, 4, c.probeServer)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, result := range results {
		name := tui.Clean(value(servers[result.index], "name", "VLESS server"))
		if result.err != nil {
			fmt.Fprintf(c.out, "  SKIP %s: %s\n", name, result.err)
		} else {
			fmt.Fprintf(c.out, "  %s %4d ms  %s\n", result.Country, result.Latency.Milliseconds(), name)
		}
	}
	if len(results) == 0 || results[0].err != nil {
		return errors.New("No verified non-RU route can reach OpenAI. Pi was not started; direct/RU fallback is disabled.")
	}
	for _, best := range results {
		if best.err != nil {
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := c.activate(servers[best.index]); err != nil {
			fmt.Fprintln(c.out, "Candidate could not start; trying the next verified route.")
			continue
		}
		// Validate the real listener too, not just the isolated benchmark tunnel.
		live, err := measureRoute(ctx, c.proxyURL(), countryURL, openAIURL, 1)
		if err != nil || !c.matchesRunning(servers[best.index]) {
			fmt.Fprintln(c.out, "Candidate failed its final exit/OpenAI check; trying the next verified route.")
			continue
		}
		cached := selection{Version: 1, Servers: digest(keys), Server: keys[best.index], Checked: time.Now(), measurement: best.measurement}
		cached.Country = live.Country
		if err := writeJSON(c.path("selection.json"), cached); err != nil {
			return errors.New("Could not save proxy selection cache.")
		}
		fmt.Fprintf(c.out, "Selected: %s, exit %s, median %d ms (cached for 30 min).\n", tui.Clean(value(servers[best.index], "name", "")), live.Country, best.Latency.Milliseconds())
		return nil
	}
	return errors.New("All candidates failed their final check; Pi was not started. Direct/RU fallback is disabled.")
}
