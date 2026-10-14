package proxy

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func transport() *http.Transport {
	return &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 20 * time.Second, IdleConnTimeout: 30 * time.Second}
}
func (c *controller) fetchSubscription(rawURL string, insecure bool) ([]string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, errors.New("Subscription download failed: invalid HTTP(S) URL.")
	}
	tr := transport()
	defer tr.CloseIdleConnections()
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // Explicit legacy flag/environment override only.
		fmt.Fprintln(c.errOut, "Warning: TLS certificate verification is DISABLED for subscription download. Use this only for trusted self-hosted subscription URLs.")
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("Subscription download failed: invalid request.")
	}
	req.Header.Set("User-Agent", "V2Box/1.0")
	resp, err := (&http.Client{Transport: tr, Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, errors.New("Subscription download failed: connection or TLS error.")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Subscription download failed: HTTP %d.", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New("Subscription download failed: could not read response.")
	}
	var links []string
	for _, line := range strings.Split(strings.ReplaceAll(decodePayload(raw), "\r", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "vless://") {
			links = append(links, line)
		}
	}
	return links, nil
}
func (c *controller) checkURL(target string) (bool, string) {
	proxyURL, _ := url.Parse(c.proxyURL())
	tr := transport()
	tr.Proxy = http.ProxyURL(proxyURL)
	defer tr.CloseIdleConnections()
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return false, "invalid request"
	}
	req.Header.Set("User-Agent", "pi-proxy-check/1.0")
	resp, err := (&http.Client{Transport: tr, Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return false, "proxy connection or TLS error"
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 100 && resp.StatusCode < 500, fmt.Sprintf("HTTP %d", resp.StatusCode)
}
func (c *controller) processStatus() error {
	unlock, err := c.lock()
	if err != nil {
		return err
	}
	defer unlock()
	pid, err := c.readPID()
	if err != nil {
		return err
	}
	running := pid > 0 && pidRunning(pid) && c.owned(pid)
	if running {
		fmt.Fprintf(c.out, "xray is running for Pi (PID %d)\n", pid)
	} else {
		fmt.Fprintln(c.out, "xray is NOT running for Pi")
		if pid > 0 {
			if pidRunning(pid) {
				fmt.Fprintf(c.errOut, "PID file points to PID %d, but it is not this pi xray process.\n", pid)
			}
			if err := c.removePID(); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c *controller) status() error {
	if err := c.processStatus(); err != nil {
		return err
	}
	for _, u := range []string{"https://api.openai.com/v1/models", "https://auth.openai.com/", "https://chatgpt.com/"} {
		ok, result := c.checkURL(u)
		label := "FAIL"
		if ok {
			label = "OK"
		}
		fmt.Fprintf(c.out, "Proxy check %s: %s -> %s\n", label, u, result)
	}
	data, err := os.ReadFile(c.path("config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("Could not read xray configuration.")
	}
	var cfg struct {
		Outbounds []struct {
			Settings struct {
				VNext []struct {
					Address string
					Port    int
				} `json:"vnext"`
			}
		}
	}
	if json.Unmarshal(data, &cfg) != nil {
		return errors.New("Invalid xray configuration.")
	}
	if len(cfg.Outbounds) > 0 && len(cfg.Outbounds[0].Settings.VNext) > 0 {
		v := cfg.Outbounds[0].Settings.VNext[0]
		fmt.Fprintf(c.out, "Current server: %s:%d\n", v.Address, v.Port)
	} else {
		fmt.Fprintln(c.out, "Current server: None:None")
	}
	return nil
}
