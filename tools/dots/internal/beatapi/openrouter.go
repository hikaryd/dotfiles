package beatapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"
)

const openRouterURL = "https://openrouter.ai/api/alpha/decisions"

func (h *handler) requestDecision(ctx context.Context, body []byte, fields map[string]json.RawMessage, cfg Config) (*http.Response, string, error) {
	if cfg.OpenRouterAPIKey != "" {
		res, err := h.requestOpenRouter(ctx, fields, cfg.OpenRouterAPIKey)
		if ctx.Err() != nil {
			if res != nil {
				res.Body.Close()
			}
			return nil, "OpenRouter", ctx.Err()
		}
		if err == nil && res.StatusCode != 408 && res.StatusCode != 429 && res.StatusCode < 500 {
			return res, "OpenRouter", nil
		}
		if res != nil {
			res.Body.Close()
		}
	}
	res, err := h.requestUpstream(ctx, "/systemone", body, cfg.APIKey)
	return res, "BeatAPI", err
}

func (h *handler) requestOpenRouter(ctx context.Context, fields map[string]json.RawMessage, key string) (*http.Response, error) {
	primary := make(map[string]json.RawMessage, len(fields))
	for name, value := range fields {
		primary[name] = value
	}
	primary["model"], _ = json.Marshal(openRouterModel)
	body, err := json.Marshal(primary)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", openRouterURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	res, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body = http.NoBody
		return res, nil
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxBody+1))
	content, _, contentErr := mime.ParseMediaType(res.Header.Get("Content-Type"))
	var payload struct {
		Answers map[string]json.RawMessage
		Error   json.RawMessage
	}
	if err != nil || len(b) > maxBody || contentErr != nil || content != "application/json" || json.Unmarshal(b, &payload) != nil || payload.Answers == nil || len(payload.Error) > 0 {
		return nil, errors.New("Invalid or interrupted OpenRouter decision response")
	}
	var questions map[string]json.RawMessage
	json.Unmarshal(fields["questions"], &questions)
	for name, rawQuestion := range questions {
		if answer := bytes.TrimSpace(payload.Answers[name]); len(answer) == 0 || bytes.Equal(answer, []byte("null")) {
			return nil, errors.New("Incomplete OpenRouter decision response")
		}
		var question struct{ Type string }
		json.Unmarshal(rawQuestion, &question)
		if question.Type == "noul" {
			var answer struct {
				Type string
				Noul *float64
			}
			if json.Unmarshal(payload.Answers[name], &answer) != nil || answer.Type != "noul" || answer.Noul == nil || *answer.Noul < 0 || *answer.Noul > 1 {
				return nil, errors.New("Invalid OpenRouter noul response")
			}
		}
	}
	// Buffer before canceling the attempt or exposing any bytes to the caller.
	res.Body = io.NopCloser(bytes.NewReader(b))
	return res, nil
}
