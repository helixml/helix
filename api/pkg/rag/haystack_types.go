package rag

// QueryRequest represents the structure of a query request to Haystack
type QueryRequest struct {
	Query   string      `json:"query"`
	Filters QueryFilter `json:"filters"`
	TopK    int         `json:"top_k"`
}

// QueryFilter represents the filter structure in a Haystack query
type QueryFilter struct {
	Operator   string      `json:"operator"`
	Conditions []Condition `json:"conditions"`
}

// Condition represents a single condition in a Haystack query filter
type Condition struct {
	Operator   string      `json:"operator,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
	Field      string      `json:"field,omitempty"`
	Value      string      `json:"value,omitempty"`
}

// QueryResponse represents the structure of a response from Haystack
type QueryResponse struct {
	Results []QueryResult `json:"results"`
}

// QueryResult represents a single result in a Haystack query response
type QueryResult struct {
	Content  string         `json:"content"`
	Metadata ResultMetadata `json:"metadata"`
	Score    float64        `json:"score"`
}

// ResultMetadata represents the metadata of a Haystack query result
type ResultMetadata struct {
	DocumentID      string            `json:"document_id"`
	DocumentGroupID string            `json:"document_group_id"`
	Source          string            `json:"source"`
	ContentOffset   int               `json:"content_offset"`
	CustomMetadata  map[string]string `json:"custom_metadata,omitempty"`
}
