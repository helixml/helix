package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// HaystackRAG implements the RAG interface using Haystack
type HaystackRAG struct {
	client   *http.Client
	endpoint string
}

// NewHaystackRAG creates a new Haystack RAG client
func NewHaystackRAG(endpoint string) *HaystackRAG {
	return &HaystackRAG{
		// Large documents take an unbounded amount of time to process.
		client:   &http.Client{Timeout: 0},
		endpoint: endpoint,
	}
}

// Index implements the RAG interface for indexing documents
// Files are sent here NOT chunked, despite the type.
func (h *HaystackRAG) Index(ctx context.Context, chunks ...*types.SessionRAGIndexChunk) error {
	logger := log.With().
		Str("component", "HaystackRAG").
		Str("method", "Index").
		Logger()

	logger.Debug().Int("chunks", len(chunks)).Msg("Indexing chunks")

	// For each chunk, create a multipart request with the content
	for _, chunk := range chunks {
		// Add debug logging to see the actual values of Source and Filename
		logger.Debug().
			Str("source", chunk.Source).
			Str("filename", chunk.Filename).
			Str("document_id", chunk.DocumentID).
			Str("document_group_id", chunk.DocumentGroupID).
			Int("content_offset", chunk.ContentOffset).
			Msg("Chunk details for indexing")

		// Create multipart form
		var b bytes.Buffer
		w := multipart.NewWriter(&b)

		// Split filename into base and extension
		base, ext := strings.TrimSuffix(
			filepath.Base(chunk.Filename),
			filepath.Ext(chunk.Filename)), strings.TrimPrefix(filepath.Ext(chunk.Filename), ".")
		filename := fmt.Sprintf("%s_%d.%s", base, chunk.ContentOffset, ext)

		logger.Debug().Str("filename", filename).Msg("Indexing file")

		fw, err := w.CreateFormFile("file", filename)
		if err != nil {
			return fmt.Errorf("creating form file: %w", err)
		}
		_, err = fw.Write([]byte(chunk.Content))
		if err != nil {
			return fmt.Errorf("writing chunk text: %w", err)
		}

		// Add metadata
		metadata := map[string]interface{}{
			"data_entity_id":    chunk.DataEntityID,
			"document_id":       chunk.DocumentID,
			"document_group_id": chunk.DocumentGroupID,
			"content_offset":    chunk.ContentOffset,
			"source":            chunk.Source,
			"original_filename": filepath.Base(chunk.Source),
			// Add other metadata as needed
		}
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshaling metadata: %w", err)
		}

		err = w.WriteField("metadata", string(metadataJSON))
		if err != nil {
			return fmt.Errorf("writing metadata field: %w", err)
		}

		// Close multipart writer
		w.Close()

		// Create request
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint+"/process", &b)
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", w.FormDataContentType())

		// Send request
		resp, err := h.client.Do(req)
		if err != nil {
			logger.Err(err).Msg("error making request to Haystack service")
			return fmt.Errorf("sending request: %w", err)
		}
		defer resp.Body.Close()

		// Check response
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			logger.Error().
				Int("status_code", resp.StatusCode).
				Str("response", string(body)).
				Msg("bad response from Haystack service")
			return fmt.Errorf("bad response: %s (%s)", resp.Status, string(body))
		}

		logger.Debug().
			Str("document_id", chunk.DocumentID).
			Str("data_entity_id", chunk.DataEntityID).
			Msg("Successfully indexed chunk")
	}

	return nil
}

// Query implements the RAG interface for querying documents
func (h *HaystackRAG) Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error) {
	logger := log.With().
		Str("component", "HaystackRAG").
		Str("method", "Query").
		Str("data_entity_id", q.DataEntityID).
		Interface("document_id_list", q.DocumentIDList).
		Logger()

	documentIDConditions := []map[string]interface{}{}
	for _, documentID := range q.DocumentIDList {
		documentIDConditions = append(documentIDConditions, map[string]interface{}{
			"field":    "meta.document_id",
			"operator": "==",
			"value":    documentID,
		})
	}
	documentIDFilter := map[string]interface{}{
		"operator":   "OR",
		"conditions": documentIDConditions,
	}

	conditions := []map[string]interface{}{}
	conditions = append(conditions, map[string]interface{}{
		"field":    "meta.data_entity_id",
		"operator": "==",
		"value":    q.DataEntityID,
	})
	if len(documentIDConditions) > 0 {
		conditions = append(conditions, documentIDFilter)
	}

	filters := map[string]interface{}{
		"operator":   "AND",
		"conditions": conditions,
	}

	log.Trace().Interface("filters", filters).Msg("Query filters")

	queryReq := map[string]interface{}{
		"query":   q.Prompt,
		"filters": filters,
		"top_k":   q.MaxResults,
	}

	bts, err := json.Marshal(queryReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling query: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint+"/query", bytes.NewReader(bts))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := h.client.Do(req)
	if err != nil {
		logger.Err(err).Msg("error making request to Haystack service")
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error().
			Int("status_code", resp.StatusCode).
			Str("response", string(body)).
			Msg("bad response from Haystack service")
		return nil, fmt.Errorf("bad response: %s (%s)", resp.Status, string(body))
	}

	// Parse response
	var queryResp struct {
		Results []struct {
			Content  string                 `json:"content"`
			Metadata map[string]interface{} `json:"metadata"`
			Meta     map[string]interface{} `json:"meta,omitempty"`
			Score    float64                `json:"score"`
		} `json:"results"`
	}

	err = json.NewDecoder(resp.Body).Decode(&queryResp)
	if err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Convert to SessionRAGResult
	results := make([]*types.SessionRAGResult, len(queryResp.Results))
	for i, r := range queryResp.Results {
		// Extract document_id and document_group_id from metadata
		documentID, _ := r.Metadata["document_id"].(string)
		documentGroupID, _ := r.Metadata["document_group_id"].(string)

		// Try to get the source from metadata with fallbacks
		var source string

		// First check meta field if it exists (Haystack stores original metadata here)
		if r.Meta != nil {
			if metaSource, ok := r.Meta["source"].(string); ok && metaSource != "" && !strings.HasPrefix(metaSource, "tmp") {
				source = metaSource
			}
		}

		// If we still don't have a source, try our other options
		if source == "" {
			// Next try original_filename which we explicitly added
			if origFilename, ok := r.Metadata["original_filename"].(string); ok && origFilename != "" {
				source = origFilename
			} else if metaSource, ok := r.Metadata["source"].(string); ok && metaSource != "" && !strings.HasPrefix(metaSource, "tmp") {
				// Then try the source from metadata if it's not a tmp file
				source = metaSource
			} else {
				// Finally fall back to regular source
				source, _ = r.Metadata["source"].(string)
			}
		}

		// Convert content_offset from float64 to int if present
		var contentOffset int
		if co, ok := r.Metadata["content_offset"].(float64); ok {
			contentOffset = int(co)
		}

		results[i] = &types.SessionRAGResult{
			Content:         r.Content,
			Distance:        r.Score,
			DocumentID:      documentID,
			DocumentGroupID: documentGroupID,
			Source:          source,
			ContentOffset:   contentOffset,
		}
	}

	logger.Debug().
		Int("results", len(results)).
		Msg("Successfully retrieved results from Haystack")

	return results, nil
}

// Delete implements the RAG interface for deleting documents
func (h *HaystackRAG) Delete(ctx context.Context, req *types.DeleteIndexRequest) error {
	logger := log.With().
		Str("component", "HaystackRAG").
		Str("method", "Delete").
		Str("data_entity_id", req.DataEntityID).
		Logger()

	logger.Debug().Msg("Deleting documents from Haystack")

	// Create delete request
	deleteReq := map[string]interface{}{
		"filters": map[string]interface{}{
			"data_entity_id": req.DataEntityID,
		},
	}

	bts, err := json.Marshal(deleteReq)
	if err != nil {
		return fmt.Errorf("marshaling delete request: %w", err)
	}

	// Create request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint+"/delete", bytes.NewReader(bts))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := h.client.Do(httpReq)
	if err != nil {
		logger.Err(err).Msg("error making request to Haystack service")
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Error().
			Int("status_code", resp.StatusCode).
			Str("response", string(body)).
			Msg("bad response from Haystack service")
		return fmt.Errorf("bad response: %s (%s)", resp.Status, string(body))
	}

	logger.Debug().Msg("Successfully deleted documents from Haystack")

	return nil
}
