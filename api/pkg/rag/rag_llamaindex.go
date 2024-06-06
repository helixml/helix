package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

const (
	defaultDistanceFunction = "cosine"
	defaultThreshold        = 0.2
	defaultMaxResults       = 3
)

// Static check
var _ RAG = &Llamaindex{}

type Llamaindex struct {
	indexURL   string
	queryURL   string
	httpClient *http.Client
}

func NewLlamaindex(indexURL, queryURL string) *Llamaindex {
	return &Llamaindex{
		indexURL:   indexURL,
		queryURL:   queryURL,
		httpClient: http.DefaultClient,
	}
}

func (l *Llamaindex) Index(ctx context.Context, indexReq *types.SessionRAGIndexChunk) error {
	logger := log.With().
		Str("llamaindex_index_url", l.indexURL).
		Logger()

	if indexReq.DataEntityID == "" {
		return fmt.Errorf("data entity ID cannot be empty")
	}

	bts, err := json.Marshal(indexReq)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.indexURL, bytes.NewReader(bts))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		logger.Err(err).Msg("error making request to llamaindex")
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Err(err).Msg("failed to read response body")
		return fmt.Errorf("error reading response body: %s", err.Error())
	}

	if resp.StatusCode >= 400 {
		logger.Err(err).Msg("bad status code from the extractor")
		return fmt.Errorf("error response from server: %s (%s)", resp.Status, string(body))
	}

	return nil
}

func (l *Llamaindex) Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error) {
	logger := log.With().
		Str("llamaindex_query_url", l.queryURL).
		Str("distance_function", q.DistanceFunction).
		Int("max_results", q.MaxResults).
		Int("distance_threshold", int(q.DistanceThreshold)).
		Logger()

	if q.Prompt == "" {
		return nil, fmt.Errorf("prompt cannot be empty")
	}

	if q.DataEntityID == "" {
		return nil, fmt.Errorf("data entity ID cannot be empty")
	}

	// Set defaults
	if q.DistanceFunction == "" {
		q.DistanceFunction = defaultDistanceFunction
	}

	if q.DistanceThreshold == 0 {
		q.DistanceThreshold = defaultThreshold
	}

	if q.MaxResults == 0 {
		q.MaxResults = defaultMaxResults
	}

	bts, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.queryURL, bytes.NewReader(bts))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		logger.Err(err).Msg("error making request to extractor")
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Err(err).Msg("failed to read response body")
		return nil, fmt.Errorf("error reading response body: %s", err.Error())
	}

	if resp.StatusCode >= 400 {
		logger.Err(err).Msg("bad status code from the llamaindex")
		return nil, fmt.Errorf("error response from server: %s (%s)", resp.Status, string(body))
	}

	var queryResp []*types.SessionRAGResult
	err = json.Unmarshal(body, &queryResp)
	if err != nil {
		return nil, fmt.Errorf("error parsing JSON (%s), error: %s", string(body), err.Error())
	}

	return queryResp, nil
}
