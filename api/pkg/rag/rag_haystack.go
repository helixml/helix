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

		// Use the original filename without adding content offset suffix
		// This prevents temporary/modified filenames from leaking into user-facing code
		filename := filepath.Base(chunk.Filename)

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

		// Add any custom metadata from the chunk
		if chunk.Metadata != nil {
			for k, v := range chunk.Metadata {
				metadata[k] = v
			}
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

	// Build document ID conditions
	documentIDConditions := make([]Condition, len(q.DocumentIDList))
	for i, documentID := range q.DocumentIDList {
		documentIDConditions[i] = Condition{
			Field:    "meta.document_id",
			Operator: "==",
			Value:    documentID,
		}
	}

	// Build the complete query request
	queryReq := QueryRequest{
		Query: q.Prompt,
		TopK:  q.MaxResults,
		Filters: QueryFilter{
			Operator: "AND",
			Conditions: []Condition{
				{
					Field:    "meta.data_entity_id",
					Operator: "==",
					Value:    q.DataEntityID,
				},
			},
		},
	}

	// Add document ID filter if there are any document IDs
	if len(documentIDConditions) > 0 {
		queryReq.Filters.Conditions = append(queryReq.Filters.Conditions, Condition{
			Operator:   "OR",
			Conditions: documentIDConditions,
		})
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
	var queryResp QueryResponse
	err = json.NewDecoder(resp.Body).Decode(&queryResp)
	if err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Convert to SessionRAGResult
	results := make([]*types.SessionRAGResult, len(queryResp.Results))
	for i, r := range queryResp.Results {
		// Use the source from metadata, falling back to filename if needed
		source := r.Metadata.Source
		if source == "" {
			// If source is empty, try to get it from the filename in metadata
			if filename, ok := r.Metadata.CustomMetadata["filename"]; ok && filename != "" {
				source = filename
			}
		}

		results[i] = &types.SessionRAGResult{
			Content:         r.Content,
			Distance:        r.Score,
			DocumentID:      r.Metadata.DocumentID,
			DocumentGroupID: r.Metadata.DocumentGroupID,
			Source:          source,
			ContentOffset:   r.Metadata.ContentOffset,
			Metadata:        r.Metadata.CustomMetadata,
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
