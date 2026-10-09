package vpn

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const defaultURL = "https://speed.cloudflare.com/__down?bytes={bytes}"

var proxyTypes = strings.Fields("socks http shadowsocks vmess trojan naive wireguard hysteria shadowtls vless tuic hysteria2 anytls snell tailcat tor ssh")

func downloadURL(template string, size int) (string, error) {
	s := strings.ReplaceAll(template, "{bytes}", strconv.Itoa(size))
	u, e := url.Parse(s)
	bad := e != nil
	if !bad {
		bad = (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != ""
		if p := u.Port(); p != "" {
			n, e := strconv.Atoi(p)
			bad = bad || e != nil || n < 1 || n > 65535
		}
	}
	bad = bad || strings.ContainsAny(s, "{}") || strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || r < 32 })
	if bad {
		return "", errors.New("URLs must be HTTP(S), without userinfo, fragments or unknown placeholders")
	}
	return s, nil
}
func freePort() (int, error) {
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return 0, errors.New("cannot reserve isolated listener")
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
func randomHex() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", errors.New("cannot generate proxy authentication")
	}
	return hex.EncodeToString(b), nil
}
func failedTrial(e string) object {
	return object{"ok": false, "http": 0, "bytes": 0, "mbps": nil, "ttfb_ms": nil, "seconds": nil, "error": e}
}
func round(n float64, places int) float64 { p := math.Pow10(places); return math.Round(n*p) / p }
func finite(n float64) bool               { return !math.IsInf(n, 0) && !math.IsNaN(n) }
func seconds(d time.Duration) string      { return strconv.FormatFloat(d.Seconds(), 'f', -1, 64) }
func measurement(ctx context.Context, port int, password string, size int, timeout time.Duration, source string) object {
	if ctx.Err() != nil {
		t := failedTrial("duration_budget_exhausted")
		t["skipped"] = true
		return t
	}
	u, e := downloadURL(source, size)
	if e != nil {
		return failedTrial("invalid_curl_metrics")
	}
	remaining := timeout + 3*time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining = min(remaining, time.Until(deadline))
	}
	curlTime := min(timeout, remaining)
	c, cancel := context.WithTimeout(ctx, remaining)
	defer cancel()
	b, code, e := command(c, fmt.Sprintf("proxy-user = %q\nurl = %q\n", "bench:"+password, u), "curl", "--disable", "--config", "-", "--globoff", "--noproxy", "", "--proxy", fmt.Sprintf("socks5h://127.0.0.1:%d", port), "--silent", "--output", "/dev/null", "--connect-timeout", seconds(min(5*time.Second, curlTime)), "--max-time", seconds(curlTime), "--write-out", "%{json}")
	if e != nil {
		if ctx.Err() != nil {
			return failedTrial("duration_budget_exhausted")
		}
		if c.Err() != nil {
			return failedTrial("curl_timeout")
		}
		return failedTrial("curl_unavailable")
	}
	return parseMetrics(b, code, size)
}
func parseMetrics(b []byte, code, size int) object {
	var m object
	if json.Unmarshal(b, &m) != nil {
		return failedTrial("invalid_curl_metrics")
	}
	nums := []float64{}
	for _, k := range []string{"http_code", "size_download", "speed_download", "time_starttransfer", "time_total"} {
		n, ok := m[k].(float64)
		if !ok || !finite(n) || n < 0 {
			return failedTrial("invalid_curl_metrics")
		}
		nums = append(nums, n)
	}
	http, downloaded, speed, ttfb, sec := nums[0], nums[1], nums[2], nums[3], nums[4]
	if sec <= 0 {
		return failedTrial("invalid_curl_metrics")
	}
	var why any
	switch {
	case code != 0:
		why = "curl_failed"
	case http != 200:
		why = "unexpected_http"
	case downloaded != float64(size):
		why = "incomplete_download"
	}
	var mbps any
	if why == nil {
		mbps = round(speed*8/1e6, 2)
	}
	return object{"ok": why == nil, "http": http, "bytes": downloaded, "mbps": mbps, "ttfb_ms": round(ttfb*1000, 1), "seconds": round(sec, 3), "curl_exit": code, "error": why}
}
func waitListener(ctx context.Context, p *child, port int, d time.Duration, exited, timeout string) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return errors.New("duration_budget_exhausted")
		case <-p.done:
			return errors.New(exited)
		case <-timer.C:
			return errors.New(timeout)
		default:
		}
		c, e := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if e == nil {
			c.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("duration_budget_exhausted")
		case <-time.After(50 * time.Millisecond):
		}
	}
}
func benchmarkNode(ctx context.Context, node object, size int, timeout time.Duration, source string) (object, error) {
	port, e := freePort()
	if e != nil {
		return nil, e
	}
	password, e := randomHex()
	if e != nil {
		return nil, e
	}
	outbound := object{}
	for k, v := range node {
		outbound[k] = v
	}
	outbound["tag"] = "bench"
	config := object{"log": object{"disabled": true}, "dns": object{"servers": []object{{"type": "local", "tag": "local"}}}, "inbounds": []object{{"type": "mixed", "listen": "127.0.0.1", "listen_port": port, "users": []object{{"username": "bench", "password": password}}}}, "outbounds": []object{outbound}, "route": object{"final": "bench", "auto_detect_interface": true, "default_domain_resolver": object{"server": "local"}}}
	dir, e := os.MkdirTemp("", "vpn-bench-")
	if e != nil {
		return nil, errors.New("isolated_process_failed")
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "config.json")
	if e = os.WriteFile(path, jsonBytes(config), 0600); e != nil {
		return nil, errors.New("isolated_process_failed")
	}
	p, e := startChild(exec.Command("sing-box", "run", "-c", path))
	if e != nil {
		return nil, errors.New("isolated_process_failed")
	}
	defer p.stop(5 * time.Second)
	if e = waitListener(ctx, p, port, 5*time.Second, "isolated_process_exited", "isolated_start_timeout"); e != nil {
		return nil, e
	}
	return measurement(ctx, port, password, size, timeout, source), nil
}
func selectCandidates(config object, only, extra []string, all bool) ([]object, error) {
	b, _ := json.Marshal(config["outbounds"])
	var outbounds []object
	if json.Unmarshal(b, &outbounds) != nil || outbounds == nil {
		return nil, errors.New("invalid sing-box outbounds or missing auto pool")
	}
	nodes := map[string]object{}
	tags := []string{}
	for _, n := range outbounds {
		tag, ok := n["tag"].(string)
		if !ok {
			return nil, errors.New("invalid sing-box outbounds or missing auto pool")
		}
		if _, ok := nodes[tag]; ok {
			return nil, errors.New("outbound tags must be unique")
		}
		nodes[tag] = n
		if all && slices.Contains(proxyTypes, str(n["type"])) {
			tags = append(tags, tag)
		}
	}
	if !all {
		if len(only) > 0 {
			tags = append(tags, only...)
		} else {
			pool, ok := nodes["auto"]["outbounds"].([]any)
			if !ok {
				return nil, errors.New("invalid sing-box outbounds or missing auto pool")
			}
			for _, v := range pool {
				s, ok := v.(string)
				if !ok {
					return nil, errors.New("invalid sing-box outbounds or missing auto pool")
				}
				tags = append(tags, s)
			}
			tags = append(tags, extra...)
		}
		tags = unique(tags)
	}
	if len(tags) == 0 || !all && len(tags) > 16 {
		return nil, errors.New("choose existing proxy tags; maximum 16 unless --all; VPS-direct is reserved")
	}
	selected := []object{{"type": "direct", "tag": "VPS-direct"}}
	for _, tag := range tags {
		n := nodes[tag]
		if n == nil || !slices.Contains(proxyTypes, str(n["type"])) || tag == "VPS-direct" {
			return nil, errors.New("choose existing proxy tags; maximum 16 unless --all; VPS-direct is reserved")
		}
		selected = append(selected, n)
	}
	return selected, nil
}
func unique(a []string) []string {
	r := []string{}
	for _, s := range a {
		if !slices.Contains(r, s) {
			r = append(r, s)
		}
	}
	return r
}
func median(a []float64) any {
	if len(a) == 0 {
		return nil
	}
	b := append([]float64{}, a...)
	sort.Float64s(b)
	if len(b)%2 == 1 {
		return b[len(b)/2]
	}
	return (b[len(b)/2-1] + b[len(b)/2]) / 2
}
func benchmarkMain(parent context.Context, args []string, out, errout io.Writer) int {
	fs := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	config := fs.String("config", "/etc/sing-box/config.json", "")
	samples := fs.Int("samples", 0, "")
	size := fs.Int("bytes", 0, "")
	timeout := fs.Int("timeout", 0, "")
	all := fs.Bool("all", false, "")
	duration := fs.Float64("max-duration", 0, "")
	var only, extra, urls stringFlags
	fs.Var(&only, "only", "")
	fs.Var(&extra, "extra", "")
	fs.Var(&urls, "url", "")
	e := fs.Parse(args)
	if errors.Is(e, flag.ErrHelp) {
		fmt.Fprintln(out, "benchmark [--config PATH] [--all|--only TAG] [--extra TAG] [--samples 1..5] [--bytes N] [--timeout 1..30] [--max-duration SECONDS] [--url URL]")
		return 0
	}
	bad := func(s string) int { fmt.Fprintln(errout, s); return 2 }
	if e != nil || len(fs.Args()) > 0 {
		return bad("invalid benchmark arguments")
	}
	if *all && len(only) > 0 {
		return bad("--all and --only are mutually exclusive")
	}
	provided := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if !provided["samples"] {
		*samples = 2
		if *all {
			*samples = 1
		}
	}
	if !provided["bytes"] {
		*size = 10000000
		if *all {
			*size = 1000000
		}
	}
	if !provided["timeout"] {
		*timeout = 20
		if *all {
			*timeout = 10
		}
	}
	if *samples < 1 || *samples > 5 || *size < 1 || *size > 50000000 || *timeout < 1 || *timeout > 30 {
		return bad("samples 1..5, bytes 1..50000000, timeout 1..30")
	}
	if provided["max-duration"] && (!finite(*duration) || *duration <= 0 || *duration > 86400) {
		return bad("max-duration must be positive and at most 86400 seconds")
	}
	if urls == nil {
		urls = []string{defaultURL}
	}
	if len(urls) > 4 {
		return bad("choose 1..4 download sources")
	}
	validated := []string{}
	for _, u := range urls {
		v, e := downloadURL(u, *size)
		if e != nil {
			return bad(e.Error())
		}
		validated = append(validated, v)
	}
	validated = unique(validated)
	b, e := os.ReadFile(*config)
	if e != nil {
		return bad("cannot read sing-box config")
	}
	var cfg object
	if json.Unmarshal(b, &cfg) != nil {
		return bad("invalid sing-box config JSON")
	}
	nodes, e := selectCandidates(cfg, only, extra, *all)
	if e != nil {
		return bad(e.Error())
	}
	return benchmarkPlan(parent, out, errout, nodes, validated, *samples, *size, *timeout, *duration)
}
func benchmarkPlan(parent context.Context, out, errout io.Writer, nodes []object, urls []string, samples, size, timeout int, duration float64) int {
	sources := []string{}
	for i, u := range urls {
		p, _ := url.Parse(u)
		sources = append(sources, fmt.Sprintf("%d:%s", i+1, p.Hostname()))
	}
	protocols := []string{}
	for _, n := range nodes {
		if n["type"] != "direct" {
			protocols = append(protocols, str(n["type"]))
		}
	}
	protocols = unique(protocols)
	sort.Strings(protocols)
	total := len(nodes) * len(sources) * samples
	worst := total * (timeout + 23)
	if duration == 0 {
		duration = float64(min(worst, 86400))
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(duration*float64(time.Second)))
	defer cancel()
	deadline, _ := ctx.Deadline()
	started := time.Now()
	emit(out, "plan", object{"proxy_nodes": len(nodes) - 1, "protocols": protocols, "sources": sources, "total_trials": total, "samples": samples, "bytes": size, "timeout": timeout, "planned_download_bytes": total * size, "duration_budget_seconds": duration, "worst_case_seconds": worst})
	results := make([][][]object, len(sources))
	for i := range results {
		results[i] = make([][]object, len(nodes))
	}
	completed, failures, skipped := 0, 0, 0
	for r := 0; r < samples; r++ {
		for si := range sources {
			s := (si + r) % len(sources)
			for ni := range nodes {
				if parent.Err() != nil {
					fmt.Fprintln(errout, "Benchmark cancelled; isolated process and temporary config removed.")
					return 130
				}
				n := (ni + r) % len(nodes)
				begin := time.Now()
				var trial object
				if ctx.Err() != nil {
					trial = failedTrial("duration_budget_exhausted")
					trial["skipped"] = true
				} else {
					var e error
					trial, e = benchmarkNode(ctx, nodes[n], size, time.Duration(timeout)*time.Second, urls[s])
					if e != nil {
						code := e.Error()
						if !slices.Contains([]string{"duration_budget_exhausted", "isolated_process_exited", "isolated_start_timeout"}, code) {
							code = "isolated_process_failed"
						}
						trial = failedTrial(code)
					}
				}
				trial["wall_seconds"] = round(time.Since(begin).Seconds(), 3)
				results[s][n] = append(results[s][n], trial)
				completed++
				if trial["skipped"] == true {
					skipped++
				} else if trial["ok"] != true {
					failures++
				}
				emit(out, "trial", object{"round": r + 1, "source": sources[s], "node": nodes[n]["tag"], "protocol": nodes[n]["type"], "trial": trial, "progress": object{"completed": completed, "total": total, "failures": failures, "skipped": skipped, "elapsed_seconds": round(time.Since(started).Seconds(), 3), "remaining_budget_seconds": round(max(0, time.Until(deadline).Seconds()), 3)}})
			}
		}
	}
	failed := false
	for s := range sources {
		for n := range nodes {
			speeds := []float64{}
			skip := 0
			for _, t := range results[s][n] {
				if t["ok"] == true {
					speeds = append(speeds, t["mbps"].(float64))
				}
				if t["skipped"] == true {
					skip++
				}
			}
			if len(speeds) == 0 {
				failed = true
			}
			emit(out, "summary", object{"source": sources[s], "node": nodes[n]["tag"], "protocol": nodes[n]["type"], "trials": results[s][n], "successful": len(speeds), "failures": len(results[s][n]) - len(speeds) - skip, "skipped": skip, "median_mbps": median(speeds)})
		}
	}
	emit(out, "complete", object{"completed": completed, "attempted": completed - skipped, "failures": failures, "skipped": skipped, "elapsed_seconds": round(time.Since(started).Seconds(), 3)})
	if parent.Err() != nil {
		return 130
	}
	if failed || skipped > 0 {
		return 1
	}
	return 0
}
