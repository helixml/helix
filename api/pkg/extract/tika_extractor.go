package extract

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/avast/retry-go/v4"
	"github.com/google/go-tika/tika"
	"github.com/rs/zerolog/log"
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
	resp, err := retry.DoWithData(func() (string, error) {
		return e.extract(ctx, extractReq)
	},
		retry.Attempts(3),
		retry.Delay(1*time.Second),
		retry.Context(ctx),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			log.Warn().
				Err(err).
				Uint("retry_number", n).
				Msg("retrying app text extraction")
		}),
	)
	return resp, err
}

func (e *TikaExtractor) extract(ctx context.Context, extractReq *ExtractRequest) (string, error) {
	if extractReq.URL == "" && len(extractReq.Content) == 0 {
		return "", fmt.Errorf("no URL or content provided")
	}

	if extractReq.URL != "" {
		resp, err := e.httpClient.Get(extractReq.URL)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		bts, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		switch {
		case strings.HasSuffix(extractReq.URL, ".pdf"):
			// PDF will be passed into Tika extractor
			extractReq.Content = bts
		// TODO: we can handle other cases that can be extracted by Tika
		default:
			// HTML will be converted to markdown
			converter := md.NewConverter("", true, nil)

			markdown, err := converter.ConvertString(string(bts))
			if err != nil {
				return "", err
			}

			return markdown, nil
		}

	}

	client := tika.NewClient(e.httpClient, e.extractorURL)

	hdr := http.Header{}
	hdr.Set("Accept", "text/plain")

	parsed, err := client.ParseWithHeader(ctx, bytes.NewReader(extractReq.Content), hdr)
	if err != nil {
		return "", err
	}

	return parsed, nil
}
