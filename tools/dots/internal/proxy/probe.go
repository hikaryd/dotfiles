package proxy

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	countryURL = "https://www.cloudflare.com/cdn-cgi/trace"
	openAIURL  = "https://api.openai.com/v1/models"
)

// ISO 3166-1 alpha-2 codes; pseudo/unknown locations must fail closed.
const isoCountries = " AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY QA RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW "

type measurement struct {
	Country string        `json:"country"`
	Latency time.Duration `json:"latency"`
}

// Only an observed exit country counts; subscription labels and entry IPs do not.
func allowedCountry(country string) bool {
	return len(country) == 2 && country[0] >= 'A' && country[0] <= 'Z' &&
		country[1] >= 'A' && country[1] <= 'Z' && country != "RU" &&
		strings.Contains(isoCountries, " "+country+" ")
}

func measureRoute(ctx context.Context, proxy, countryTarget, apiTarget string, samples int) (measurement, error) {
	var result measurement
	u, err := url.Parse(proxy)
	if err != nil {
		return result, errors.New("invalid proxy URL")
	}
	tr := transport()
	tr.Proxy = http.ProxyURL(u) // Never inherit NO_PROXY, even for explicit bypasses.
	defer tr.CloseIdleConnections()
	client := &http.Client{
		Transport: tr, Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	get := func(target string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, errors.New("invalid probe request")
		}
		req.Header.Set("User-Agent", "pi-proxy-check/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return nil, errors.New("proxy connection or TLS error")
		}
		return resp, nil
	}
	resp, err := get(countryTarget)
	if err != nil {
		return result, err
	}
	if resp.StatusCode == http.StatusOK {
		scanner := bufio.NewScanner(io.LimitReader(resp.Body, 8192))
		for scanner.Scan() {
			if country, ok := strings.CutPrefix(scanner.Text(), "loc="); ok {
				result.Country = strings.TrimSpace(country)
			}
		}
		if scanner.Err() != nil {
			result.Country = ""
		}
	}
	resp.Body.Close()
	if !allowedCountry(result.Country) {
		return result, errors.New("exit is RU or country could not be verified")
	}
	latencies := make([]time.Duration, 0, samples)
	for i := 0; i < samples; i++ {
		// Benchmark fresh CONNECT/TLS requests, not a lucky warm connection.
		tr.CloseIdleConnections()
		started := time.Now()
		resp, err = get(apiTarget)
		if err != nil {
			return result, err
		}
		elapsed := time.Since(started)
		resp.Body.Close()
		// 401 is the expected unauthenticated OpenAI response. In particular,
		// 403 (region block / challenge), redirects and 429 are not availability.
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
			return result, errors.New("OpenAI unavailable (HTTP " + strconv.Itoa(resp.StatusCode) + ")")
		}
		latencies = append(latencies, elapsed)
	}
	result.Latency = medianLatency(latencies)
	return result, nil
}

func (c *controller) measureRoute(ctx context.Context, proxy string, samples int) (measurement, error) {
	country, api := c.countryTarget, c.apiTarget
	if country == "" {
		country = countryURL
	}
	if api == "" {
		api = openAIURL
	}
	return measureRoute(ctx, proxy, country, api, samples)
}

func (c *controller) probeServer(ctx context.Context, server object, samples int) (measurement, error) {
	return c.withProbe(ctx, server, func(ctx context.Context, proxy string) (measurement, error) {
		return c.measureRoute(ctx, proxy, samples)
	})
}

func (c *controller) withProbe(ctx context.Context, server object, check func(context.Context, string) (measurement, error)) (measurement, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var result measurement
	bin, err := exec.LookPath("xray")
	if err != nil {
		return result, errors.New("xray not found (brew install xray)")
	}
	dir, err := os.MkdirTemp(c.dir, ".probe-")
	if err != nil {
		return result, errors.New("could not create private probe directory")
	}
	defer os.RemoveAll(dir)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return result, errors.New("could not allocate probe port")
	}
	probe := *c
	probe.port = listener.Addr().(*net.TCPAddr).Port
	cfg, err := probe.config(server)
	if err == nil {
		err = writeJSON(dir+"/config.json", cfg)
	}
	listener.Close()
	if err != nil {
		return result, errors.New("invalid probe configuration")
	}
	cmd := exec.CommandContext(ctx, bin, "run", "-c", dir+"/config.json")
	// xray output can contain credentials; never attach it to diagnostics.
	if err := cmd.Start(); err != nil {
		return result, errors.New("could not start probe xray")
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	defer func() { _ = cmd.Process.Kill(); <-done }()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		conn, err := (&net.Dialer{Timeout: 50 * time.Millisecond}).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(probe.port)))
		if err == nil {
			conn.Close()
			return check(ctx, probe.proxyURL())
		}
		select {
		case <-done:
			return result, errors.New("probe xray exited (diagnostics withheld)")
		case <-ctx.Done():
			return result, ctx.Err()
		case <-ticker.C:
		}
	}
}
