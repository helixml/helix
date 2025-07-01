package searxng

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

type SearchProvider interface {
	Search(ctx context.Context, req *SearchRequest) ([]SearchResultItem, error)
}

const DefaultMaxResults = 10

type Category = string

const (
	EmptyCategory       Category = ""
	GeneralCategory     Category = "general"
	NewsCategory        Category = "news"
	SocialMediaCategory Category = "social_media"
)

type SearchResultItem struct {
	// URL The URL of the search result
	URL string `json:"url"`
	// Title The title of the search result
	Title string `json:"title"`
	// Content The content snippet of the search result
	Content string `json:"content,omitempty"`
	// Query The query used to obtain this search result
	Query string `json:"query"`
	// Category: Category of the search queries.
	Category Category `json:"category,omitempty"`
	// Metadata search result metadata
	Metadata string `json:"metadata,omitempty"`
	// PublishedDate The published date of the search result
	PublishedDate string `json:"publishedDate,omitempty"`
	// Score search result score
	Score float64 `json:"score,omitempty"`
}

// Output represents the output of the SearxNG search tool.
// the schema implements SystemPromptContextProvider
type Output struct {
	// Query The query used to obtain this search result
	Query string `json:"query,omitempty"`
	// Results List of search result items
	Results []SearchResultItem `json:"results,omitempty"`
	// Category The category of the search results
	Category Category `json:"category,omitempty"`
}

type SearXNG struct {
	httpClient *http.Client
	baseURL    string
}

type Config struct {
	BaseURL string // Server URL
}

// SearchRequest is the request to the SearxNG API
type SearchRequest struct {
	MaxResults int
	Queries    []SearchQuery // List of queries to search for
}

type SearchQuery struct {
	Query    string   // The query to search for
	Category Category // The category of the query
	Language string   // The language of the query
}

func NewSearXNG(cfg *Config) *SearXNG {
	return &SearXNG{
		httpClient: http.DefaultClient,
		baseURL:    cfg.BaseURL,
	}
}

func (s *SearXNG) Search(ctx context.Context, req *SearchRequest) ([]SearchResultItem, error) {
	if req.MaxResults == 0 {
		req.MaxResults = DefaultMaxResults
	}

	pool := pool.New().WithErrors()

	resultsMu := sync.Mutex{}
	var results []SearchResultItem

	for _, q := range req.Queries {
		pool.Go(func() error {
			queryResults, err := s.fetchSearchResults(ctx, &q)
			if err != nil {
				return err
			}
			resultsMu.Lock()
			results = append(results, queryResults...)
			resultsMu.Unlock()
			return nil
		})
	}

	err := pool.Wait()
	if err != nil {
		log.Error().Err(err).Msg("error searching with searxng")
	}

	// Sort results by score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Remove duplicates
	seen := make(map[string]bool)
	uniqueResults := make([]SearchResultItem, 0, len(results))
	for _, r := range results {
		if !seen[r.URL] {
			seen[r.URL] = true
			uniqueResults = append(uniqueResults, r)
		}
	}

	// Limit results
	if len(uniqueResults) > req.MaxResults {
		uniqueResults = uniqueResults[:req.MaxResults]
	}

	return uniqueResults, nil
}

func (s *SearXNG) fetchSearchResults(ctx context.Context, q *SearchQuery) ([]SearchResultItem, error) {
	// Encode the query parameter
	values := url.Values{}
	values.Set("q", q.Query)
	values.Set("safesearch", "0")
	values.Set("format", "json")
	values.Set("engines", "bing,duckduckgo,google,startpage,yandex")
	if q.Language != "" {
		values.Set("language", q.Language)
	}
	if q.Category != EmptyCategory {
		values.Set("categories", q.Category)
	}
	searchURL := fmt.Sprintf("%s/search?%s", s.baseURL, values.Encode())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, err
	}

	// Add User-Agent header to avoid 403 errors
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (compatible; HelixBot/1.0)")

	httpResp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error querying local search engine: %v", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		bts, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading response body: %v", err)
		}
		return nil, fmt.Errorf("non-200 response from search engine: %d: %s", httpResp.StatusCode, string(bts))
	}

	var searchResult Output
	if err := json.NewDecoder(httpResp.Body).Decode(&searchResult); err != nil {
		return nil, err
	}
	for idx := range searchResult.Results {
		searchResult.Results[idx].Query = q.Query
	}

	return searchResult.Results, nil
}
