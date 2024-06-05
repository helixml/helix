package extract

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog/log"
)

type ExtractRequest struct {
	URL     string `json:"url"`
	Content []byte `json:"content"`
}

type Extractor interface {
	Extract(ctx context.Context, req *ExtractRequest) (string, error)
}

// DefaultExtractor is the default, llamaindex based text extractor
// that can download URLs and uses unstructured.io under the hood
type DefaultExtractor struct {
	extractorURL string
	httpClient   *http.Client
}

func NewDefaultExtractor(extractorURL string) *DefaultExtractor {
	return &DefaultExtractor{
		extractorURL: extractorURL,
		httpClient:   http.DefaultClient,
	}
}

func (e *DefaultExtractor) Extract(ctx context.Context, extractReq *ExtractRequest) (string, error) {
	if extractReq.URL == "" && len(extractReq.Content) == 0 {
		return "", fmt.Errorf("no URL or content provided")
	}

	// TODO: check if this is just normal text, if yes
	// then don't send it to the extractor as we can
	// just use it as is

	var content string

	// If content is set, base64 encode it before sending
	if len(extractReq.Content) > 0 {
		content = base64.StdEncoding.EncodeToString(extractReq.Content)
	}

	logger := log.With().
		Str("url", extractReq.URL).
		Int("content_length", len(extractReq.Content)).
		Str("extractor_url", e.extractorURL).
		Logger()

	bts, err := json.Marshal(&llamaindexExtractRequest{
		URL:     extractReq.URL,
		Content: content,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.extractorURL, bytes.NewReader(bts))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		logger.Err(err).Msg("error making request to extractor")
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Err(err).Msg("failed to read response body")
		return "", fmt.Errorf("error reading response body: %s", err.Error())
	}

	if resp.StatusCode >= 400 {
		logger.Err(err).Msg("bad status code from the extractor")
		return "", fmt.Errorf("error response from server: %s (%s)", resp.Status, string(body))
	}

	var extractResp llamaindexExtractResponse
	err = json.Unmarshal(body, &extractResp)
	if err != nil {
		return "", fmt.Errorf("error parsing JSON (%s), error: %s", string(body), err.Error())
	}

	return extractResp.Text, nil
}

type llamaindexExtractRequest struct {
	URL     string `json:"url"`
	Content string `json:"content"`
}

type llamaindexExtractResponse struct {
	Text string `json:"text"`
}
