package beatapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const upstreamURL = "https://api.beatapi.io/v1"
const maxBody = 10 << 20

type handler struct {
	load     func() (Config, error)
	client   *http.Client
	upstream string
	inflight chan struct{}
}

func newHandler(load func() (Config, error), client *http.Client, upstream string) http.Handler {
	return &handler{load: load, client: client, upstream: strings.TrimRight(upstream, "/"), inflight: make(chan struct{}, 8)}
}
func upstreamClient(proxy string) (*http.Client, error) {
	u, err := url.Parse(proxy)
	if err != nil {
		return nil, errors.New("beatapi: invalid outgoing proxy")
	}
	tr := &http.Transport{Proxy: http.ProxyURL(u), DialContext: (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 60 * time.Second, MaxIdleConns: 16, MaxIdleConnsPerHost: 8, ForceAttemptHTTP2: true}
	return &http.Client{Transport: tr, Timeout: 10 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}
func newID(prefix string) string {
	var b [16]byte
	rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}
func localRequest(r *http.Request) bool {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host != "127.0.0.1" && host != "localhost" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "http" || u.Host != r.Host || u.User != nil || u.Path != "" {
			return false
		}
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, anthropic bool, status int, message string) {
	typ := "api_error"
	switch status {
	case 400, 413, 415:
		typ = "invalid_request_error"
	case 401:
		typ = "authentication_error"
	case 403:
		typ = "permission_error"
	case 429:
		typ = "rate_limit_error"
	}
	e := map[string]any{"error": map[string]string{"type": typ, "message": message}}
	if anthropic {
		e["type"] = "error"
	}
	writeJSON(w, status, e)
}
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	anthropic := r.URL.Path == "/v1/messages"
	if !localRequest(r) {
		apiError(w, anthropic, 403, "Only loopback clients and same-origin requests are allowed")
		return
	}
	if r.URL.RawQuery != "" {
		apiError(w, anthropic, 400, "Query parameters are unsupported")
		return
	}
	path := r.URL.Path
	expected := "POST"
	if path == "/health" || path == "/v1/models" {
		expected = "GET"
	} else if path != "/v1/systemone" && path != "/v1/chat/completions" && !anthropic {
		apiError(w, anthropic, 404, "Unknown API endpoint")
		return
	}
	if r.Method != expected {
		w.Header().Set("Allow", expected)
		apiError(w, anthropic, 405, "Method not allowed")
		return
	}
	c, err := h.load()
	if err != nil {
		apiError(w, anthropic, 503, "Secure configuration unavailable; run setup or key")
		return
	}
	if path == "/health" {
		primary, fallback := jevModel, ""
		if c.OpenRouterAPIKey != "" {
			primary, fallback = openRouterModel, jevModel
		}
		writeJSON(w, 200, map[string]any{"service": label, "version": "2", "configured": true, "capabilities": []string{"beatapi", "openrouter"}, "jev_primary": primary, "jev_fallback": fallback, "endpoints": []string{"/v1/models", "/v1/chat/completions", "/v1/messages", "/v1/systemone"}})
		return
	}
	if path == "/v1/models" {
		writeJSON(w, 200, map[string]any{"object": "list", "data": listedModels()})
		return
	}
	ct, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || ct != "application/json" {
		apiError(w, anthropic, 415, "Content-Type must be application/json")
		return
	}
	select {
	case h.inflight <- struct{}{}:
		defer func() { <-h.inflight }()
	default:
		apiError(w, anthropic, 429, "Local inference capacity reached; try later")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			apiError(w, anthropic, 413, "Request body exceeds 10 MiB")
		} else {
			apiError(w, anthropic, 400, "Cannot read request body")
		}
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || fields == nil {
		apiError(w, anthropic, 400, "Invalid JSON object")
		return
	}
	var model string
	if json.Unmarshal(fields["model"], &model) != nil {
		apiError(w, anthropic, 400, errModel.Error())
		return
	}
	resolved, err := resolveModel(model, path == "/v1/systemone")
	if err != nil {
		apiError(w, anthropic, 400, err.Error())
		return
	}
	if resolved != model {
		fields["model"], _ = json.Marshal(resolved)
		body, _ = json.Marshal(fields)
	}
	if anthropic {
		body, err = anthropicRequest(body)
		if err != nil {
			apiError(w, true, 400, "Unsupported Anthropic request; only text and tool blocks are supported")
			return
		}
	}
	provider := "BeatAPI"
	var res *http.Response
	if path == "/v1/systemone" {
		res, provider, err = h.requestDecision(r.Context(), body, fields, c)
		w.Header().Set("X-Dots-Jev-Provider", provider)
		if c.OpenRouterAPIKey != "" && provider == "BeatAPI" {
			w.Header().Set("X-Dots-Jev-Fallback", "true")
		}
	} else {
		res, err = h.requestUpstream(r.Context(), "/chat/completions", body, c.APIKey)
	}
	if err != nil {
		status := 502
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			status = 504
		}
		apiError(w, anthropic, status, provider+" transport unavailable or timed out; check the outgoing proxy")
		return
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if retry := res.Header.Get("Retry-After"); len(retry) < 128 {
			if seconds, err := strconv.ParseUint(retry, 10, 32); err == nil {
				w.Header().Set("Retry-After", strconv.FormatUint(seconds, 10))
			} else if date, err := http.ParseTime(retry); err == nil {
				w.Header().Set("Retry-After", date.UTC().Format(http.TimeFormat))
			}
		}
		message := upstreamRejection(res.StatusCode)
		if provider == "OpenRouter" {
			message = openRouterRejection(res.StatusCode)
		}
		apiError(w, anthropic, res.StatusCode, message)
		return
	}
	if anthropic {
		var streaming bool
		json.Unmarshal(fields["stream"], &streaming)
		if streaming {
			if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
				apiError(w, true, 502, "BeatAPI did not return a stream")
				return
			}
			if err := streamAnthropic(w, res.Body, model); err != nil {
				sseEvent(w, "error", map[string]any{"type": "error", "error": map[string]string{"type": "api_error", "message": "BeatAPI stream interrupted"}})
			}
			return
		}
		b, readErr := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
		if readErr != nil || len(b) > maxBody {
			apiError(w, true, 502, "BeatAPI response unreadable or too large")
			return
		}
		b, err = anthropicResponse(b, model)
		if err != nil {
			apiError(w, true, 502, "Invalid BeatAPI completion")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(res.StatusCode)
		if writeChunk(w, b) != nil {
			panic(http.ErrAbortHandler)
		}
		return
	}
	content, _, parseErr := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if res.Header.Get("Content-Type") == "" {
		content, parseErr = "application/json", nil
	}
	if parseErr != nil || (content != "application/json" && content != "text/event-stream") {
		apiError(w, false, 502, "Unsupported BeatAPI response content type")
		return
	}
	w.Header().Set("Content-Type", content)
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	if content == "text/event-stream" {
		w.WriteHeader(res.StatusCode)
		if relaySSE(w, res.Body) != nil {
			panic(http.ErrAbortHandler)
		}
		return
	}
	b, readErr := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	var payload struct{ Error json.RawMessage }
	if readErr != nil || len(b) > maxBody || json.Unmarshal(b, &payload) != nil || len(payload.Error) > 0 {
		apiError(w, false, 502, "Invalid or interrupted BeatAPI JSON response")
		return
	}
	w.WriteHeader(res.StatusCode)
	if writeChunk(w, b) != nil {
		panic(http.ErrAbortHandler)
	}
}

func writeChunk(w http.ResponseWriter, b []byte) error {
	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
	if _, err := w.Write(b); err != nil {
		return err
	}
	if err := rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	return nil
}
