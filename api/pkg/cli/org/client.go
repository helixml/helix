package org

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
)

// httpClient is a thin authenticated HTTP helper for helix-org REST paths
// that are not (yet) on the typed Go client. Prefer NewClientFromEnv for
// org resolution; use this for /orgs/{org}/bots|topics|processors|….
type httpClient struct {
	base   string // e.g. http://localhost:8080/api/v1
	apiKey string
	http   *http.Client
}

func newHTTPClient() (*httpClient, error) {
	c, err := client.NewClientFromEnv()
	if err != nil {
		return nil, err
	}
	// HelixClient.url already includes /api/v1 — reach it via a small
	// parallel helper that exposes the same env defaults.
	url := os.Getenv("HELIX_URL")
	if url == "" {
		url = "http://localhost:8080"
	}
	url = strings.TrimRight(url, "/")
	if !strings.HasSuffix(url, "/api/v1") {
		url = url + "/api/v1"
	}
	apiKey := os.Getenv("HELIX_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("HELIX_API_KEY is not set")
	}
	_ = c
	return &httpClient{
		base:   url,
		apiKey: apiKey,
		http:   &http.Client{Timeout: 0}, // per-request timeouts
	}, nil
}

func (c *httpClient) resolveOrg(ctx context.Context, orgFlag string) (string, error) {
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return "", err
	}
	// Prefer HELIX_ORG when flag empty.
	if orgFlag == "" {
		orgFlag = os.Getenv("HELIX_ORG")
	}
	return cli.ResolveOrganization(ctx, apiClient, orgFlag)
}

// doJSON performs method path with optional JSON body and decodes into out
// when non-nil. path is relative to /api/v1 (e.g. "/orgs/foo/bots").
func (c *httpClient) doJSON(ctx context.Context, method, path string, body any, out any, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var rdr io.Reader
	if body != nil {
		bts, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(bts)
	}
	full := c.base + path
	req, err := http.NewRequestWithContext(reqCtx, method, full, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil || len(raw) == 0 || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s: %w (body: %s)", path, err, truncate(string(raw), 200))
	}
	return nil
}

// doRaw returns status + body for the escape-hatch api command.
func (c *httpClient) doRaw(ctx context.Context, method, path string, body []byte, timeout time.Duration) (int, []byte, http.Header, error) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	// Allow absolute /api/v1/... or path-only /orgs/...
	full := path
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		full = path
	} else if strings.HasPrefix(path, "/api/v1") {
		// strip host base which already ends in /api/v1
		baseHost := strings.TrimSuffix(c.base, "/api/v1")
		full = baseHost + path
	} else if strings.HasPrefix(path, "/") {
		full = c.base + path
	} else {
		full = c.base + "/" + path
	}

	req, err := http.NewRequestWithContext(reqCtx, method, full, rdr)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, resp.Header, err
	}
	return resp.StatusCode, raw, resp.Header, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// printJSON pretty-prints v (or raw) to stdout.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printJSONRaw(raw []byte) error {
	var pretty any
	if err := json.Unmarshal(raw, &pretty); err != nil {
		_, err2 := os.Stdout.Write(raw)
		if !strings.HasSuffix(string(raw), "\n") {
			fmt.Println()
		}
		return err2
	}
	return printJSON(pretty)
}

// PrintJSONRaw is the exported form of printJSONRaw for the api package.
func PrintJSONRaw(raw []byte) error { return printJSONRaw(raw) }

// DoRawAPI is the exported escape-hatch used by `helix api`.
func DoRawAPI(ctx context.Context, method, path string, body []byte, timeout time.Duration) (int, []byte, error) {
	c, err := newHTTPClient()
	if err != nil {
		return 0, nil, err
	}
	status, raw, _, err := c.doRaw(ctx, method, path, body, timeout)
	return status, raw, err
}
