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
	"strconv"

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
		Str("component", "haystack_rag").
		Int("chunk_count", len(chunks)).
		Logger()

	logger.Debug().Msg("Indexing documents")

	for _, chunk := range chunks {
		logger.Debug().
			Str("data_entity_id", chunk.DataEntityID).
			Str("document_id", chunk.DocumentID).
			Msg("indexing chunk")

		// Metadata check before processing
		if chunk.Metadata != nil {
			logger.Info().
				Str("document_id", chunk.DocumentID).
				Interface("chunk_metadata", chunk.Metadata).
				Msg("Chunk contains metadata")
		} else {
			logger.Info().
				Str("document_id", chunk.DocumentID).
				Msg("Chunk does NOT contain metadata")
		}

		var b bytes.Buffer
		w := multipart.NewWriter(&b)

		// Use the original filename without adding content offset suffix
		// This prevents temporary/modified filenames from leaking into user-facing code
		filename := filepath.Base(chunk.Filename)

		logger.Debug().Str("filename", filename).Msg("Indexing file")

		// Add the file as a part
		part, err := w.CreateFormFile("file", filename)
		if err != nil {
			return fmt.Errorf("creating form file: %w", err)
		}

		_, err = part.Write([]byte(chunk.Content))
		if err != nil {
			return fmt.Errorf("writing file content: %w", err)
		}

		// Add metadata for the document
		metadata := map[string]string{
			"document_id":       chunk.DocumentID,
			"document_group_id": chunk.DocumentGroupID,
			"source":            chunk.Source,
			"content_offset":    fmt.Sprintf("%d", chunk.ContentOffset),
			"filename":          filename,
			"original_filename": filepath.Base(chunk.Source),
			"data_entity_id":    chunk.DataEntityID,
			// Add other metadata as needed
		}

		// Add any custom metadata from the chunk
		if chunk.Metadata != nil {
			logger.Info().
				Str("document_id", chunk.DocumentID).
				Interface("chunk_metadata", chunk.Metadata).
				Msg("Adding metadata to document during indexing")

			// First add all user metadata
			for k, v := range chunk.Metadata {
				// Skip if it's a critical system field to prevent overriding
				if k == "document_id" || k == "document_group_id" || k == "source" ||
					k == "content_offset" || k == "filename" || k == "original_filename" ||
					k == "data_entity_id" {
					logger.Info().
						Str("document_id", chunk.DocumentID).
						Str("key", k).
						Str("value", v).
						Msg("Skipping user metadata key - system field takes precedence")
					continue
				}

				logger.Info().
					Str("document_id", chunk.DocumentID).
					Str("key", k).
					Str("value", v).
					Msg("Adding metadata key-value to document")
				metadata[k] = v
			}
		}

		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshaling metadata: %w", err)
		}

		logger.Info().
			Str("document_id", chunk.DocumentID).
			Str("metadataJSON", string(metadataJSON)).
			Msg("Metadata being sent to Haystack")

		metadataPart, err := w.CreateFormField("metadata")
		if err != nil {
			return fmt.Errorf("creating metadata field: %w", err)
		}

		_, err = metadataPart.Write(metadataJSON)
		if err != nil {
			return fmt.Errorf("writing metadata: %w", err)
		}

		// Create the form field for data_entity_id
		dataEntityPart, err := w.CreateFormField("data_entity_id")
		if err != nil {
			return fmt.Errorf("creating data_entity_id field: %w", err)
		}
		_, err = dataEntityPart.Write([]byte(chunk.DataEntityID))
		if err != nil {
			return fmt.Errorf("writing data_entity_id: %w", err)
		}

		w.Close()

		// Create the request
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.endpoint+"/process", &b)
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", w.FormDataContentType())

		// Send the request
		resp, err := h.client.Do(req)
		if err != nil {
			logger.Err(err).Msg("error making request to Haystack service")
			return fmt.Errorf("sending request: %w", err)
		}
		defer resp.Body.Close()

		// Check the response
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode >= 400 {
			return fmt.Errorf("error response from server: %s (%s)", resp.Status, string(body))
		}

		logger.Debug().
			Str("document_id", chunk.DocumentID).
			Str("response", string(body)).
			Msg("document indexed successfully")
	}

	logger.Debug().Msg("All documents indexed successfully")
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
		var source string
		if sourceVal, ok := r.Metadata["source"]; ok {
			source = toString(sourceVal)
		}

		if source == "" {
			// If source is empty, try to get it from the filename in metadata
			if filenameVal, ok := r.Metadata["filename"]; ok {
				source = toString(filenameVal)
			}
		}

		// Add debug logging to see what metadata we're getting back
		logger.Info().
			Str("document_id", toString(r.Metadata["document_id"])).
			Interface("metadata", r.Metadata).
			Msg("Retrieved document with metadata")

		// Get document ID and group ID from metadata
		documentID := toString(r.Metadata["document_id"])
		documentGroupID := toString(r.Metadata["document_group_id"])

		// Convert metadata to string values for SessionRAGResult
		metadata := make(map[string]string)
		for k, v := range r.Metadata {
			metadata[k] = toString(v)
		}

		// Create result with all metadata fields
		results[i] = &types.SessionRAGResult{
			Content:         r.Content,
			Distance:        r.Score,
			DocumentID:      documentID,
			DocumentGroupID: documentGroupID,
			Source:          source,
			ContentOffset:   parseContentOffset(toString(r.Metadata["content_offset"])),
			Metadata:        metadata,
		}

		// Check for source_url in metadata
		if sourceURLVal, ok := r.Metadata["source_url"]; ok {
			sourceURL := toString(sourceURLVal)
			if sourceURL != "" {
				logger.Info().
					Str("document_id", documentID).
					Str("source_url", sourceURL).
					Msg("Found source_url in document metadata")
			}
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

// parseContentOffset safely converts a string contentOffset to an integer
func parseContentOffset(offset string) int {
	if offset == "" {
		return 0
	}

	intOffset, err := strconv.Atoi(offset)
	if err != nil {
		log.Warn().
			Str("offset", offset).
			Err(err).
			Msg("Failed to parse ContentOffset as integer, using 0")
		return 0
	}

	return intOffset
}

// toString safely converts an interface{} value to a string
func toString(value interface{}) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		// For any other type, try to use fmt.Sprint
		return fmt.Sprint(v)
	}
}
