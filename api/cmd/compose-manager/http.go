package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// getJSON fetches a JSON body and decodes it into T. If allow404 is true,
// a 404 response returns (nil, nil) — used for "is there an assignment?"
// where 404 is a normal state.
func getJSON[T any](ctx context.Context, c *http.Client, url, token string, allow404 bool) (*T, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound && allow404 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("GET %s: %s — %s", url, resp.Status, string(body))
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode %T: %w", out, err)
	}
	return &out, nil
}

var _ = errors.New // keep errors imported if future helpers want it
