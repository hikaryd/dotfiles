package beatapi

import (
	"bytes"
	"context"
	"net/http"
)

func (h *handler) requestUpstream(ctx context.Context, endpoint string, body []byte, key string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", h.upstream+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	return h.client.Do(req)
}
