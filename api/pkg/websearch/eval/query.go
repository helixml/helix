// Package eval measures the relevance of web search results. It runs a frozen
// query set against Helix, compares it with a frozen capture of Exa's results
// for the same requests, and grades every result with a frozen LLM judge, so
// scores from different rounds of ranking work are comparable.
package eval

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/helixml/helix/api/pkg/websearch"
)

// TextChars is how much page text each search requests per result.
const TextChars = 1500

type Query struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Query      string `json:"query"`
	Category   string `json:"category,omitempty"`
	NumResults int    `json:"numResults"`
}

// Request is the /search request sent for the query, identical for every
// provider.
func (q Query) Request() *websearch.SearchRequest {
	return &websearch.SearchRequest{
		Query:      q.Query,
		NumResults: q.NumResults,
		Category:   q.Category,
		Contents: &websearch.ContentsOptions{Text: &websearch.TextOption{
			Enabled: true, TextOptions: websearch.TextOptions{MaxCharacters: TextChars},
		}},
	}
}

func LoadQueries(path string) ([]Query, error) {
	var queries []Query
	if err := readJSON(path, &queries); err != nil {
		return nil, err
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("%s: no queries", path)
	}
	seen := map[string]bool{}
	for _, q := range queries {
		if q.ID == "" || q.Kind == "" || q.Query == "" || q.NumResults <= 0 {
			return nil, fmt.Errorf("%s: query %q needs id, kind, query and numResults", path, q.ID)
		}
		if seen[q.ID] {
			return nil, fmt.Errorf("%s: duplicate query id %q", path, q.ID)
		}
		seen[q.ID] = true
	}
	return queries, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Files in the eval data directory. The queries and the reference are frozen
// for a series; the judgments only grow.
const (
	QueriesFile   = "queries.json"
	ReferenceFile = "exa_reference.json"
	JudgmentsFile = "judgments.json"
)
