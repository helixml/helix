package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/rs/zerolog/log"
)

// HaystackExtractor is an implementation of the Extractor interface
// that uses Haystack for document extraction
type HaystackExtractor struct {
	endpoint   string
	httpClient *http.Client
}

// NewHaystackExtractor creates a new Haystack extractor
func NewHaystackExtractor(endpoint string) *HaystackExtractor {
	return &HaystackExtractor{
		endpoint:   endpoint,
		httpClient: http.DefaultClient,
	}
}

// Extract implements the Extractor interface for Haystack
func (e *HaystackExtractor) Extract(ctx context.Context, extractReq *Request) (string, error) {
	if extractReq.URL == "" && len(extractReq.Content) == 0 {
		return "", fmt.Errorf("no URL or content provided")
	}

	logger := log.With().
		Str("url", extractReq.URL).
		Int("content_length", len(extractReq.Content)).
		Str("extractor_url", e.endpoint).
		Logger()

	// Create multipart form
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	// XXX we are hard-coding document.pdf here.

	// If we have content, add it as a file
	if len(extractReq.Content) > 0 {
		fw, err := w.CreateFormFile("file", "document.pdf")
		if err != nil {
			return "", fmt.Errorf("creating form file: %w", err)
		}
		_, err = fw.Write(extractReq.Content)
		if err != nil {
			return "", fmt.Errorf("writing file content: %w", err)
		}
	} else if extractReq.URL != "" {
		// Add URL field
		err := w.WriteField("url", extractReq.URL)
		if err != nil {
			return "", fmt.Errorf("writing URL field: %w", err)
		}
	}

	// Close multipart writer
	w.Close()

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+"/extract", &b)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	// Send request
	resp, err := e.httpClient.Do(req)
	if err != nil {
		logger.Err(err).Msg("error making request to Haystack service")
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Err(err).Msg("failed to read response body")
		return "", fmt.Errorf("error reading response body: %s", err.Error())
	}

	// Check response status
	if resp.StatusCode >= 400 {
		logger.Err(err).Msg("bad status code from the extractor")
		return "", fmt.Errorf("error response from server: %s (%s)", resp.Status, string(body))
	}

	// Parse response
	var extractResp struct {
		Text string `json:"text"`
	}
	err = json.Unmarshal(body, &extractResp)
	if err != nil {
		return "", fmt.Errorf("error parsing JSON (%s), error: %s", string(body), err.Error())
	}

	logger.Debug().
		Int("extracted_length", len(extractResp.Text)).
		Msg("extracted text")

	return extractResp.Text, nil
}
