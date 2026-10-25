package beatapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
)

// requestUpstream keeps paid Jev internal to the free-first systemone route.
// Eligible HTTP errors or transport timeouts can trigger one paid attempt.
// Response body failures never do, and caller cancellation must never cause
// additional spending.
func (h *handler) requestUpstream(ctx context.Context, endpoint string, body []byte, key string) (*http.Response, error) {
	res, err := h.sendUpstream(ctx, endpoint, body, key)
	if endpoint != "/systemone" || ctx.Err() != nil {
		return res, err
	}
	retry := res != nil && (res.StatusCode == http.StatusTooManyRequests || (res.StatusCode >= 500 && res.StatusCode <= 599))
	if err != nil {
		var timeout net.Error
		retry = errors.As(err, &timeout) && timeout.Timeout()
	}
	if !retry {
		return res, err
	}
	var fields map[string]json.RawMessage
	var model string
	if json.Unmarshal(body, &fields) != nil || json.Unmarshal(fields["model"], &model) != nil || model != jevModel {
		return res, err
	}
	// RawMessage preserves typed fields, unknown options and numeric precision.
	fields["model"] = json.RawMessage(`"jev-1.14"`)
	paidBody, _ := json.Marshal(fields)
	if res != nil {
		res.Body.Close()
	}
	return h.sendUpstream(ctx, endpoint, paidBody, key)
}

func (h *handler) sendUpstream(ctx context.Context, endpoint string, body []byte, key string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", h.upstream+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	return h.client.Do(req)
}
