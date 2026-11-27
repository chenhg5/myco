package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// TestModels is an explicit, user-requested connection test. Background model
// discovery must use FetchModels: it never generates billable completions.
// MiniMax installations without model discovery are verified by one bounded
// completion on the selected endpoint, then offered the built-in catalogue.
func TestModels(ctx context.Context, client *http.Client, e Endpoint) (ModelList, error) {
	ms, err, unsupported := discoverModels(ctx, client, e)
	if err == nil {
		return ModelList{Models: ms, From: "provider"}, nil
	}
	v, known := VendorOf(e.Base)
	if !known || v.ID != "minimax" || !unsupported || ctx.Err() != nil || len(v.Models) == 0 {
		return ModelList{}, err
	}
	if err = testCompletion(ctx, modelClient(client), e, v.Models[0]); err != nil {
		return ModelList{}, err
	}
	ms = make([]Model, len(v.Models))
	for i, id := range v.Models {
		ms[i] = Model{ID: id}
	}
	return ModelList{Models: ms, From: "catalog"}, nil
}

func modelListUnavailable(err error) bool {
	if errors.Is(err, errNoModels) {
		return true
	}
	var status *HTTPStatusError
	if errors.As(err, &status) {
		switch status.Status {
		case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented, http.StatusUnauthorized, http.StatusForbidden:
			return true
		}
	}
	return false
}

func testCompletion(ctx context.Context, c *http.Client, e Endpoint, model string) error {
	base := strings.TrimRight(e.Base, "/")
	path := "/chat/completions"
	if e.Anthropic {
		path = "/messages"
		if !versionSegment.MatchString(base) {
			base += "/v1"
		}
	}
	body, _ := json.Marshal(map[string]any{"model": model, "max_tokens": 16,
		"messages": []map[string]string{{"role": "user", "content": "Hi"}}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("models: bad endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "myco")
	req.Header.Set("Authorization", "Bearer "+e.Key)
	if e.Anthropic {
		req.Header.Set("x-api-key", e.Key)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	res, err := c.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("models from %s: unreachable", e.Host())
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
		return &HTTPStatusError{Host: e.Host(), Status: res.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return fmt.Errorf("models from %s: invalid test response", e.Host())
	}
	// HTTP 200 alone is insufficient: some gateways return API errors in it.
	var response struct {
		ID      string            `json:"id"`
		Type    string            `json:"type"`
		Role    string            `json:"role"`
		Content []json.RawMessage `json:"content"`
		Choices []struct {
			Message struct {
				Role string `json:"role"`
			} `json:"message"`
		} `json:"choices"`
	}
	valid := json.Unmarshal(data, &response) == nil
	if e.Anthropic {
		valid = valid && response.ID != "" && response.Type == "message" && response.Role == "assistant" && response.Content != nil
	} else {
		valid = valid && len(response.Choices) > 0 && response.Choices[0].Message.Role == "assistant"
	}
	if !valid {
		return fmt.Errorf("models from %s: invalid test response", e.Host())
	}
	return nil
}
