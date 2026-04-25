package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/helixml/helix-org/server"
)

type apiError struct {
	Status int
	Title  string
	Detail string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("api error (%d): %s — %s", e.Status, e.Title, e.Detail)
}

type apiErrorsEnvelope struct {
	Errors []server.Error `json:"errors"`
}

// postJSON sends body to url and decodes either a Resource envelope or an
// errors envelope. The returned json.RawMessage is the `data` field.
func postJSON(ctx context.Context, url string, body any) (json.RawMessage, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", server.MediaType)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", url, err)
	}
	defer func() { _ = res.Body.Close() }()

	payload, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if res.StatusCode >= 400 {
		var env apiErrorsEnvelope
		if jerr := json.Unmarshal(payload, &env); jerr == nil && len(env.Errors) > 0 {
			return nil, &apiError{Status: res.StatusCode, Title: env.Errors[0].Title, Detail: env.Errors[0].Detail}
		}
		return nil, &apiError{Status: res.StatusCode, Title: "unknown", Detail: string(payload)}
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return envelope.Data, nil
}
