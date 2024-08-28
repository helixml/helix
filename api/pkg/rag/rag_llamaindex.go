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
	DefaultDistanceFunction = "cosine"
	DefaultThreshold        = 0.4
	DefaultMaxResults       = 3

	DefaultChunkSize     = 2048
	DefaultChunkOverflow = 20
)

// Static check
var _ RAG = &Llamaindex{}

type Llamaindex struct {
	indexURL   string
	queryURL   string
	deleteURL  string
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
		Str("data_entity_id", indexReq.DataEntityID).
		Str("document_group_id", indexReq.DocumentGroupID).
		Str("document_id", indexReq.DocumentID).
		Int("content_offset", indexReq.ContentOffset).
		Str("filename", indexReq.Filename).
		Str("source", indexReq.Source).
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

	logger.Info().Msg("indexed document chunk")

	return nil
}

func (l *Llamaindex) Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error) {
	logger := log.With().
		Str("llamaindex_query_url", l.queryURL).
		Str("distance_function", q.DistanceFunction).
		Int("max_results", q.MaxResults).
		Int("distance_threshold", int(q.DistanceThreshold)).
		Str("data_entity_id", q.DataEntityID).
		Logger()

	if q.Prompt == "" {
		return nil, fmt.Errorf("prompt cannot be empty")
	}

	if q.DataEntityID == "" {
		return nil, fmt.Errorf("data entity ID cannot be empty")
	}

	// Set defaults
	if q.DistanceFunction == "" {
		q.DistanceFunction = DefaultDistanceFunction
	}

	if q.DistanceThreshold == 0 {
		q.DistanceThreshold = DefaultThreshold
	}

	if q.MaxResults == 0 {
		q.MaxResults = DefaultMaxResults
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

	for _, result := range queryResp {
		// Backwards compatibility
		if result.Source == "" {
			result.Source = result.Filename
		}
	}

	logger.Info().Msg("query results")

	return queryResp, nil
}

func (l *Llamaindex) Delete(ctx context.Context, r *types.DeleteIndexRequest) error {
	logger := log.With().
		Str("llamaindex_delete_url", l.deleteURL).
		Str("data_entity_id", r.DataEntityID).
		Logger()

	if r.DataEntityID == "" {
		return fmt.Errorf("data entity ID cannot be empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, l.deleteURL+"/"+r.DataEntityID, http.NoBody)
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
		logger.Err(err).Msg("bad status code from the llamaindex")
		return fmt.Errorf("error response from server: %s (%s)", resp.Status, string(body))
	}

	logger.Info().Msg("deleted data entity")

	return nil
}
