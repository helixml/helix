package extract

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-tika/tika"
)

// TikaExtractor is the default, llamaindex based text extractor
// that can download URLs and uses unstructured.io under the hood
type TikaExtractor struct {
	extractorURL string
	httpClient   *http.Client
}

func NewTikaExtractor(extractorURL string) *TikaExtractor {
	if extractorURL == "" {
		extractorURL = "http://localhost:9998"
	}

	return &TikaExtractor{
		extractorURL: extractorURL,
		httpClient:   http.DefaultClient,
	}
}

func (e *TikaExtractor) Extract(ctx context.Context, extractReq *ExtractRequest) (string, error) {
	if extractReq.URL == "" && len(extractReq.Content) == 0 {
		return "", fmt.Errorf("no URL or content provided")
	}

	client := tika.NewClient(e.httpClient, e.extractorURL)

	parsed, err := client.Parse(ctx, bytes.NewReader(extractReq.Content))
	if err != nil {
		return "", err
	}

	return parsed, nil
}
