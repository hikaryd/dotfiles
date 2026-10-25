package beatapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const beatKeyMessage = "BeatAPI rejected the API key; run dots-tools proxy beatapi key"
const beatQuotaMessage = "BeatAPI usage limit reached; wait for Retry-After or check account limits"
const beatRejectionMessage = "BeatAPI rejected the request; check model availability, key and quota"

var errNotConnected = errors.New("beatapi: jevgrep is not connected to this API")

// Never reflect upstream messages or headers.
func upstreamRejection(status int) string {
	switch status {
	case 401:
		return beatKeyMessage
	case 429:
		return beatQuotaMessage
	default:
		return beatRejectionMessage
	}
}

func jevgrepMatches(b []byte, cfg Config) bool {
	var current struct{ Provider, BaseURL, Model, APIKey string }
	if json.Unmarshal(b, &current) != nil {
		return false
	}
	// Identify local jg even when its API configuration is missing or invalid.
	// This only selects a bounded diagnostic, never an inference destination.
	if cfg.Port == 0 {
		u, err := url.Parse(current.BaseURL)
		if err != nil {
			return false
		}
		cfg.Port, err = strconv.Atoi(u.Port())
		if err != nil || cfg.Port < 1 || cfg.Port > 65535 {
			return false
		}
	}
	return current.Provider == "custom" && current.BaseURL == "http://"+cfg.address()+"/v1" && current.Model == jevModel && current.APIKey == "local"
}

func (c *cli) jevgrepConnected(cfg Config) bool {
	path := filepath.Join(c.base, "jevgrep/credentials.json")
	if privateFile(path) != nil {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	return err == nil && len(b) <= 1<<20 && jevgrepMatches(b, cfg)
}

// One real synthetic evaluation; unlike jg 0.8, never sleep through Retry-After.
func (c *cli) doctor(cfg Config) error {
	fmt.Fprintln(c.out, "Checking Jev through the local API (one local request, 15s limit; paid fallback may be used)...")
	ctx, cancel := context.WithTimeout(c.ctx, 15*time.Second)
	defer cancel()
	body := fmt.Sprintf(`{"model":%q,"state":{"source":"export function recordEvent(event) { events.push(event); }"},"questions":{"relevant":{"type":"noul","instructions":"Does this source implement recording an event?"}}}`, jevModel)
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://"+cfg.address()+"/v1/systemone", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	res, err := client.Do(req)
	if err != nil {
		return errors.New("beatapi: Jev check failed or timed out; check local status and the outgoing proxy")
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 64<<10+1))
	if err != nil || len(b) > 64<<10 {
		return errors.New("beatapi: Jev response unreadable or too large")
	}
	if res.StatusCode != 200 {
		var payload struct{ Error struct{ Message string } }
		json.Unmarshal(b, &payload)
		message := "check model access, key and quota"
		switch payload.Error.Message {
		case beatKeyMessage, beatQuotaMessage, beatRejectionMessage:
			message = payload.Error.Message
		}
		fmt.Fprintf(c.out, "Jev check failed: HTTP %d; %s\n", res.StatusCode, message)
		if seconds, err := strconv.ParseUint(res.Header.Get("Retry-After"), 10, 32); err == nil {
			fmt.Fprintf(c.out, "Retry after %d seconds. No retry was scheduled.\n", seconds)
		} else if at, err := http.ParseTime(res.Header.Get("Retry-After")); err == nil {
			fmt.Fprintf(c.out, "Retry after %s. No retry was scheduled.\n", at.UTC().Format(http.TimeFormat))
		}
		return errors.New("beatapi: upstream access is not verified")
	}
	var payload struct {
		Answers map[string]struct {
			Type string
			Noul float64
		}
	}
	if json.Unmarshal(b, &payload) != nil {
		return errors.New("beatapi: invalid Jev response")
	}
	answer := payload.Answers["relevant"]
	if answer.Type != "noul" || answer.Noul <= 0.5 || answer.Noul > 1 {
		return errors.New("beatapi: invalid Jev response; synthetic question did not pass")
	}
	fmt.Fprintln(c.out, "Jev access verified through the local API.")
	return nil
}
