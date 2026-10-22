package vpn

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const xrayVersion = "26.3.27"
const xrayURL = "https://github.com/XTLS/Xray-core/releases/download/v26.3.27/Xray-linux-64.zip"
const xraySHA = "23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae"

func validatedURLs(templates []string, size int) ([]string, error) {
	if len(templates) == 0 {
		templates = []string{defaultURL}
	}
	if len(templates) > 4 {
		return nil, errors.New("invalid_source_count")
	}
	urls := []string{}
	for _, s := range templates {
		u, e := downloadURL(s, size)
		if e != nil || !strings.HasPrefix(u, "https://") || strings.Count(s, "{bytes}") > 1 || strings.ContainsFunc(u, func(r rune) bool { return unicode.IsSpace(r) || r <= 32 || r == 127 }) {
			return nil, errors.New("invalid_https_url_template")
		}
		urls = append(urls, u)
	}
	return urls, nil
}
func parseXHTTP(line string) (*url.URL, map[string]string, error) {
	u, e := url.Parse(strings.TrimSpace(line))
	if e != nil {
		return nil, nil, errors.New("source_node_parse_failed")
	}
	m := map[string]string{}
	for _, p := range queryPairs(u.RawQuery, true) {
		if _, exists := m[p[0]]; exists {
			return nil, nil, errors.New("duplicate_query_parameters")
		}
		m[p[0]] = p[1]
	}
	if u.Scheme != "vless" || (m["type"] != "xhttp" && m["type"] != "splithttp") {
		return nil, nil, errors.New("source_node_not_vless_xhttp")
	}
	if u.Hostname() == "" || u.User == nil || u.User.Username() == "" {
		return nil, nil, errors.New("invalid_vless_authority")
	}
	if _, has := u.User.Password(); has {
		return nil, nil, errors.New("invalid_vless_authority")
	}
	p, e := strconv.Atoi(u.Port())
	if e != nil || p < 1 || p > 65535 {
		return nil, nil, errors.New("invalid_vless_port")
	}
	return u, m, nil
}
func loadXHTTP(ctx context.Context, index int, confPath string) (*url.URL, map[string]string, error) {
	m, e := loadConf(confPath)
	if e != nil || m["SUB_URL"] == "" {
		return nil, nil, errors.New("source_node_parse_failed")
	}
	b, e := readURL(ctx, &http.Client{Timeout: 30 * time.Second}, m["SUB_URL"], 5<<20, "sing-box")
	if e != nil {
		return nil, nil, e
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if !utf8.Valid(b) || index < 0 || index >= len(lines) {
		return nil, nil, errors.New("source_node_parse_failed")
	}
	return parseXHTTP(lines[index])
}
func buildXHTTP(u *url.URL, q map[string]string) (object, error) {
	enc := value(q, "encryption", "none")
	flow := q["flow"]
	if enc == "" {
		return nil, errors.New("empty_vless_encryption")
	}
	if !slices.Contains([]string{"", "xtls-rprx-vision", "xtls-rprx-vision-udp443"}, flow) {
		return nil, errors.New("unsupported_vless_flow")
	}
	if flow != "" && enc == "none" {
		return nil, errors.New("vision_incompatible_with_xhttp_none")
	}
	mode := value(q, "mode", "auto")
	if !slices.Contains([]string{"auto", "packet-up", "stream-up", "stream-one"}, mode) {
		return nil, errors.New("invalid_xhttp_mode")
	}
	path := value(q, "path", "/")
	if path == "" {
		return nil, errors.New("empty_xhttp_path")
	}
	x := object{"mode": mode, "path": path}
	if h, ok := q["host"]; ok {
		if h == "" {
			return nil, errors.New("empty_xhttp_host")
		}
		x["host"] = h
	}
	if extra, ok := q["extra"]; ok {
		var v any
		if json.Unmarshal([]byte(extra), &v) != nil {
			return nil, errors.New("invalid_xhttp_extra_json")
		}
		if obj(v) == nil {
			return nil, errors.New("xhttp_extra_not_object")
		}
		x["extra"] = v
	}
	security := value(q, "security", "none")
	stream := object{"network": "xhttp", "security": security, "xhttpSettings": x}
	switch security {
	case "reality":
		if q["pbk"] == "" || q["fp"] == "" {
			return nil, errors.New("missing_reality_password_or_fingerprint")
		}
		r := object{"serverName": value(q, "sni", u.Hostname()), "fingerprint": q["fp"], "password": q["pbk"], "shortId": q["sid"]}
		if r["serverName"] == "" {
			return nil, errors.New("empty_reality_server_name")
		}
		for k, v := range map[string]string{"spx": "spiderX", "pqv": "mldsa65Verify"} {
			if s, ok := q[k]; ok {
				r[v] = s
			}
		}
		stream["realitySettings"] = r
	case "tls":
		t := object{"serverName": value(q, "sni", u.Hostname()), "fingerprint": value(q, "fp", "chrome")}
		if s, ok := q["alpn"]; ok {
			t["alpn"] = strings.Split(s, ",")
		}
		for k, v := range map[string]string{"ech": "echConfigList", "pcs": "pinnedPeerCertSha256", "vcn": "verifyPeerCertByName"} {
			if s, ok := q[k]; ok {
				t[v] = s
			}
		}
		stream["tlsSettings"] = t
	case "none":
	default:
		return nil, errors.New("unsupported_transport_security")
	}
	port, _ := strconv.Atoi(u.Port())
	return object{"tag": "xhttp-source-node", "protocol": "vless", "settings": object{"address": u.Hostname(), "port": port, "id": u.User.Username(), "encryption": enc, "flow": flow}, "streamSettings": stream}, nil
}

// Cache paths work without the original source checkout on a deployed Linux host.
func xrayCache() string {
	if p := os.Getenv("VPN_XRAY_CACHE"); p != "" {
		return p
	}
	if os.Geteuid() == 0 {
		return "/var/cache/dots-vpn"
	}
	if p, e := os.UserCacheDir(); e == nil {
		return filepath.Join(p, "dots-vpn")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("dots-vpn-%d", os.Geteuid()))
}
func noSymlinks(path string) bool {
	abs, e := filepath.Abs(path)
	if e != nil {
		return false
	}
	for p := abs; ; p = filepath.Dir(p) {
		s, e := os.Lstat(p)
		if e == nil && s.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return false
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return true
}
func ensureXray(ctx context.Context, base, assetURL, checksum string) (string, error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return "", errors.New("linux_x64_required")
	}
	return installXray(ctx, base, assetURL, checksum)
}
func installXray(ctx context.Context, base, assetURL, checksum string) (string, error) {
	directory := filepath.Join(base, "xray-"+xrayVersion)
	if !noSymlinks(directory) {
		return "", errors.New("unsafe_binary_directory")
	}
	if e := os.MkdirAll(directory, 0700); e != nil {
		return "", errors.New("unsafe_binary_directory")
	}
	info, e := os.Stat(directory)
	if e != nil || info.Mode().Perm()&0022 != 0 {
		return "", errors.New("unsafe_binary_directory")
	}
	archive := filepath.Join(directory, "Xray-linux-64.zip")
	s, e := os.Lstat(archive)
	var data []byte
	if e == nil {
		if !s.Mode().IsRegular() {
			return "", errors.New("unsafe_archive_path")
		}
		if s.Size() > 30<<20 {
			return "", errors.New("source_too_large")
		}
		data, e = os.ReadFile(archive)
	} else if errors.Is(e, os.ErrNotExist) {
		data, e = readURL(ctx, &http.Client{Timeout: 30 * time.Second}, assetURL, 30<<20, "isolated-xhttp-diagnostics")
	}
	if e != nil {
		return "", errors.New("source_fetch_failed")
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != checksum {
		return "", errors.New("archive_sha256_mismatch")
	}
	if s == nil {
		f, e := os.OpenFile(archive, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return "", errors.New("unsafe_archive_path")
		}
		_, we := f.Write(data)
		ce := f.Close()
		if we != nil || ce != nil {
			os.Remove(archive)
			return "", errors.New("archive_write_failed")
		}
	}
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return "", errors.New("archive_invalid")
	}
	var binaryData []byte
	for _, entry := range z.File {
		if entry.Name != "xray" {
			continue
		}
		if entry.UncompressedSize64 > 100<<20 {
			return "", errors.New("binary_too_large")
		}
		r, e := entry.Open()
		if e != nil {
			return "", errors.New("archive_invalid")
		}
		binaryData, e = io.ReadAll(io.LimitReader(r, 100<<20+1))
		r.Close()
		if e != nil {
			return "", errors.New("archive_invalid")
		}
		if len(binaryData) > 100<<20 {
			return "", errors.New("binary_too_large")
		}
		break
	}
	if binaryData == nil {
		return "", errors.New("archive_invalid")
	}
	binary := filepath.Join(directory, "xray")
	if s, e := os.Lstat(binary); e == nil && !s.Mode().IsRegular() {
		return "", errors.New("unsafe_binary_path")
	}
	existing, _ := os.ReadFile(binary)
	if !bytes.Equal(existing, binaryData) {
		if e = atomicWrite(binary, binaryData, 0700); e != nil {
			return "", errors.New("binary_install_failed")
		}
	}
	if os.Chmod(binary, 0700) != nil {
		return "", errors.New("binary_install_failed")
	}
	c, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, n, e := command(c, "", binary, "version")
	if e != nil {
		return "", errors.New("binary_version_failed")
	}
	if n != 0 || !regexp.MustCompile(`Xray 26\.3\.27(?:\s|$)`).Match(b) {
		return "", errors.New("binary_version_mismatch")
	}
	return binary, nil
}

type xhttpArgs struct {
	index, size, samples int
	timeout, duration    float64
	urls                 stringFlags
	install, check       bool
}

func parseXMetrics(output []byte, code, size, sample, source int, elapsed float64, why string) object {
	status, downloaded := 0, 0
	var transfer, speed any
	fields := strings.Split(strings.TrimSpace(string(output)), "\t")
	valid := len(fields) >= 4
	var secs, rate float64
	if valid {
		var e1, e2, e3, e4 error
		var n float64
		status, e1 = strconv.Atoi(fields[0])
		n, e2 = strconv.ParseFloat(fields[1], 64)
		secs, e3 = strconv.ParseFloat(fields[2], 64)
		rate, e4 = strconv.ParseFloat(fields[3], 64)
		valid = e1 == nil && e2 == nil && e3 == nil && e4 == nil && finite(n) && n >= 0 && n == math.Trunc(n) && n < float64(math.MaxInt64) && finite(secs) && finite(rate) && secs > 0 && rate > 0
		if finite(n) && n >= 0 && n < float64(math.MaxInt64) {
			downloaded = int(n)
		}
		if finite(secs) {
			transfer = secs
		}
	}
	if !valid && why == "" {
		why = "invalid_curl_metrics"
	}
	if why == "" {
		switch {
		case code != 0:
			why = "curl_request_failed"
		case status != 200:
			why = "unexpected_http_status"
		case downloaded != size:
			why = "unexpected_body_size"
		}
	}
	var mbps, errValue any
	if why == "" {
		speed = rate
		mbps = round(float64(downloaded)*8/secs/1e6, 6)
	} else {
		errValue = why
	}
	return object{"sample": sample, "source_id": source, "success": why == "", "http_status": status, "bytes": downloaded, "expected_bytes": size, "curl_exit": code, "wall_seconds": round(elapsed, 6), "transfer_seconds": transfer, "speed_bytes_per_second": speed, "mbps": mbps, "error": errValue}
}
func xhttpTrial(ctx context.Context, port int, auth [2]string, url string, a xhttpArgs, dir string, sample, source int) (object, error) {
	if ctx.Err() != nil {
		return nil, errors.New("deadline_exceeded")
	}
	timeout := time.Duration(a.timeout * float64(time.Second))
	if d, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(d))
	}
	c, cancel := context.WithTimeout(ctx, timeout+time.Second)
	defer cancel()
	log, e := os.OpenFile(filepath.Join(dir, fmt.Sprintf("curl-%d-%d.log", sample, source)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, errors.New("curl_process_failed")
	}
	defer log.Close()
	cmd := exec.Command("curl", "--disable", "--globoff", "--config", "-", "--silent", "--show-error", "--fail", "--retry", "0", "--noproxy", "", "--socks5-hostname", fmt.Sprintf("127.0.0.1:%d", port), "--proto", "=https", "--proto-redir", "=https", "--connect-timeout", seconds(min(10*time.Second, timeout)), "--max-time", seconds(timeout), "--output", "/dev/null", "--write-out", "%{http_code}\t%{size_download}\t%{time_total}\t%{speed_download}\n")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("proxy-user = %q\nurl = %q\n", auth[0]+":"+auth[1], url))
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = log
	begin := time.Now()
	p, e := startChild(cmd)
	code := -1
	why := ""
	if e != nil {
		why = "curl_process_failed"
	} else {
		select {
		case <-p.done:
			code = p.cmd.ProcessState.ExitCode()
		case <-c.Done():
			why = "trial_timeout"
		}
		p.stop(3 * time.Second)
		code = p.cmd.ProcessState.ExitCode()
	}
	return parseXMetrics(output.Bytes(), code, a.size, sample, source, time.Since(begin).Seconds(), why), nil
}
func xhttpSession(ctx context.Context, out io.Writer, a xhttpArgs, binary string, u *url.URL, q map[string]string, urls []string) (int, error) {
	outbound, e := buildXHTTP(u, q)
	if e != nil {
		return 1, e
	}
	dir, e := os.MkdirTemp("", ".xhttp-private-")
	if e != nil {
		return 1, errors.New("private_directory_failed")
	}
	defer os.RemoveAll(dir)
	port, e := freePort()
	if e != nil {
		return 1, e
	}
	user, e := randomHex()
	if e != nil {
		return 1, e
	}
	pass, e := randomHex()
	if e != nil {
		return 1, e
	}
	config := object{"log": object{"access": "none", "error": filepath.Join(dir, "xray.log"), "loglevel": "warning"}, "inbounds": []object{{"tag": "bench-socks", "listen": "127.0.0.1", "port": port, "protocol": "socks", "settings": object{"auth": "password", "udp": false, "accounts": []object{{"user": user, "pass": pass}}}}}, "outbounds": []object{outbound}}
	path := filepath.Join(dir, "config.json")
	if e = os.WriteFile(path, jsonBytes(config), 0600); e != nil {
		return 1, errors.New("private_config_failed")
	}
	logFile, e := os.OpenFile(filepath.Join(dir, "xray.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return 1, errors.New("private_config_failed")
	}
	logFile.Close()
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	_, code, e := command(c, "", binary, "run", "-test", "-c", path)
	cancel()
	if e != nil {
		return 1, errors.New("xray_config_check_failed")
	}
	if code != 0 {
		return 1, errors.New("xray_config_invalid")
	}
	if a.check {
		emit(out, "checked", object{"source_index": a.index, "version": xrayVersion, "protocol": "vless", "transport": "xhttp", "security": value(q, "security", "none"), "mode": value(q, "mode", "auto"), "handshake_tested": false})
		return 0, nil
	}
	emit(out, "plan", object{"source_index": a.index, "version": xrayVersion, "protocol": "vless", "transport": "xhttp", "samples": a.samples, "bytes_per_sample": a.size, "sources": len(urls), "trials": a.samples * len(urls), "timeout_seconds": a.timeout, "max_duration_seconds": a.duration})
	log, e := os.OpenFile(filepath.Join(dir, "process.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return 1, errors.New("private_log_failed")
	}
	defer log.Close()
	cmd := exec.Command(binary, "run", "-c", path)
	cmd.Stdout = log
	cmd.Stderr = log
	p, e := startChild(cmd)
	if e != nil {
		return 1, errors.New("xray_start_failed")
	}
	defer p.stop(3 * time.Second)
	if e = waitListener(ctx, p, port, 8*time.Second, "xray_exited_before_listening", "xray_startup_timeout"); e != nil {
		return 1, e
	}
	results := []object{}
	for sample := 1; sample <= a.samples; sample++ {
		for i, url := range urls {
			if ctx.Err() != nil {
				break
			}
			select {
			case <-p.done:
				return 1, errors.New("xray_exited_during_benchmark")
			default:
			}
			r, e := xhttpTrial(ctx, port, [2]string{user, pass}, url, a, dir, sample, i+1)
			if e != nil {
				return 1, e
			}
			results = append(results, r)
			row := object{"source_index": a.index}
			for k, v := range r {
				row[k] = v
			}
			emit(out, "trial", row)
		}
	}
	p.stop(3 * time.Second)
	summary := func(selected []object, expected int) object {
		speeds := []float64{}
		for _, r := range selected {
			if r["success"] == true {
				speeds = append(speeds, r["mbps"].(float64))
			}
		}
		var low, high any
		if len(speeds) > 0 {
			low = slices.Min(speeds)
			high = slices.Max(speeds)
		}
		return object{"source_index": a.index, "completed": len(selected), "successful": len(speeds), "failed": len(selected) - len(speeds), "skipped": expected - len(selected), "median_mbps": median(speeds), "min_mbps": low, "max_mbps": high}
	}
	s := summary(results, a.samples*len(urls))
	emit(out, "summary", s)
	for i := range urls {
		selected := []object{}
		for _, r := range results {
			if r["source_id"] == i+1 {
				selected = append(selected, r)
			}
		}
		s := summary(selected, a.samples)
		s["source_id"] = i + 1
		emit(out, "source_summary", s)
	}
	if s["successful"] == a.samples*len(urls) {
		return 0, nil
	}
	return 1, nil
}
func xhttpMain(parent context.Context, args []string, out, errout io.Writer) int {
	a := xhttpArgs{}
	fs := flag.NewFlagSet("xhttp-benchmark", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.IntVar(&a.index, "source-index", 6, "")
	fs.IntVar(&a.size, "bytes", 10000000, "")
	fs.IntVar(&a.samples, "samples", 3, "")
	fs.Float64Var(&a.timeout, "timeout", 20, "")
	fs.Float64Var(&a.duration, "max-duration", 240, "")
	fs.Var(&a.urls, "url", "")
	fs.Var(&a.urls, "url-template", "")
	fs.BoolVar(&a.install, "install-only", false, "")
	fs.BoolVar(&a.check, "check-only", false, "")
	e := fs.Parse(args)
	if errors.Is(e, flag.ErrHelp) {
		fmt.Fprintln(out, "xhttp-benchmark [--source-index N] [--bytes N] [--samples N] [--timeout SECONDS] [--max-duration SECONDS] [--url HTTPS_URL] [--install-only|--check-only]")
		return 0
	}
	if e != nil || len(fs.Args()) > 0 || a.install && a.check || a.index < 0 || a.size < 1 || a.size > 50000000 || a.samples < 1 || a.samples > 5 || !finite(a.timeout) || !finite(a.duration) || a.timeout <= 0 || a.timeout > 30 || a.duration <= 0 || a.duration > 86400 {
		fmt.Fprintln(errout, "invalid positive limits or source index")
		return 2
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, time.Duration(a.duration*float64(time.Second)))
	defer cancel()
	code := 1
	urls, e := validatedURLs(a.urls, a.size)
	binary := ""
	if e == nil {
		binary, e = ensureXray(ctx, xrayCache(), xrayURL, xraySHA)
	}
	if e == nil {
		if a.install {
			emit(out, "installed", object{"version": xrayVersion, "archive_sha256": xraySHA, "binary": binary})
			code = 0
		} else {
			var u *url.URL
			var q map[string]string
			u, q, e = loadXHTTP(ctx, a.index, env("VPN_ENV", "/etc/sing-box/vpn.env"))
			if e == nil {
				code, e = xhttpSession(ctx, out, a, binary, u, q, urls)
			}
		}
	}
	if parent.Err() != nil {
		e = errors.New("interrupted")
		code = 1
	}
	if e != nil {
		emit(out, "error", object{"source_index": a.index, "error": e.Error()})
	}
	emit(out, "complete", object{"source_index": a.index, "exit_code": code, "elapsed_seconds": round(time.Since(started).Seconds(), 6)})
	return code
}
