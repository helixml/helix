// Package native implements websearch.Provider on Helix's own stack:
// SearXNG finds candidate pages, a guarded HTTP client or headless Chrome
// fetches them, and Helix's LLM providers rank, summarise and answer.
package native

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/websearch"
)

//go:generate mockgen -source $GOFILE -destination components_mocks.go -package $GOPACKAGE

// Discoverer finds candidate pages for a search. It owns translating Exa
// search filters into engine queries and post-filtering what comes back.
type Discoverer interface {
	Discover(ctx context.Context, req *DiscoverRequest) (*DiscoverResult, error)
}

type DiscoverRequest struct {
	// Queries are run together and merged; the first is the user's query.
	Queries            []string
	NumResults         int
	Category           string
	IncludeDomains     []string
	ExcludeDomains     []string
	StartPublishedDate *time.Time
	EndPublishedDate   *time.Time
	Country            string // ISO 3166-1 alpha-2, may be empty
	Moderation         bool
}

// DiscoverResult lists at most NumResults candidates, best first, one per
// page (near-duplicate URLs are collapsed).
type DiscoverResult struct {
	Candidates []Candidate
	Warnings   []websearch.Warning
}

type Candidate struct {
	URL           string // normalised by websearch.NormalizeURL
	Title         string
	Snippet       string
	PublishedDate *time.Time
	Image         string
}

// RawPage is a fetched document before extraction: what the cache stores.
type RawPage struct {
	URL         string // the URL that was requested, normalised
	FinalURL    string // after redirects
	StatusCode  int
	ContentType string
	Body        []byte
	FetchedAt   time.Time
}

// Fetcher retrieves a page from the web. A failure to load this page (the
// origin, the network, the egress policy, the timeout) is *FetchError and is
// reported for that URL alone; any other error (the browser is unavailable,
// ctx is cancelled) fails the request.
type Fetcher interface {
	Fetch(ctx context.Context, url string, timeout time.Duration) (*RawPage, error)
}

// FetchError carries the content status tag reported for a URL.
type FetchError struct {
	Tag            string // websearch.Tag* content error tag
	HTTPStatusCode int
	Err            error
}

func (e *FetchError) Error() string { return e.Tag + ": " + e.Err.Error() }
func (e *FetchError) Unwrap() error { return e.Err }

// Extractor turns a raw page into text and metadata. opts have been
// validated by the request. A failure caused by the page itself (a type it
// cannot read, content that does not parse as its type) wraps
// ErrUnsupportedDocument; a failure of the document service on this page
// wraps ErrExtractionFailed. Both are reported for that URL alone; any other
// error (ctx is done) fails the request.
type Extractor interface {
	Extract(ctx context.Context, page *RawPage, opts websearch.TextOptions) (*Page, error)
}

// ErrUnsupportedDocument is an Extractor failure specific to one page,
// reported as websearch.TagUnsupportedDocument.
var ErrUnsupportedDocument = errors.New("unsupported document")

// ErrExtractionFailed is the document service failing on one page,
// reported as websearch.TagExtractionFailed: one PDF the service cannot
// handle, or the service being down, costs that result its text, not the
// whole search.
var ErrExtractionFailed = errors.New("document extraction failed")

type Page struct {
	Title         string
	Author        string
	PublishedDate *time.Time
	Image         string
	Favicon       string
	// Text is Markdown (or HTML when IncludeHTMLTags), untruncated.
	Text       string
	Links      []Link
	ImageLinks []websearch.ImageLink
}

type Link struct {
	URL    string
	Anchor string
}

// PageCache stores raw pages by normalised URL. Get returns (nil, nil) on a
// miss.
type PageCache interface {
	Get(ctx context.Context, url string) (*RawPage, error)
	Put(ctx context.Context, page *RawPage) error
}

// Ranker scores passages against a query by meaning.
type Ranker interface {
	// Rank returns one similarity score in [0,1] per passage.
	Rank(ctx context.Context, query string, passages []string) ([]float64, error)
}

// Tier selects which configured model an LLM task runs on.
type Tier string

const (
	TierFast    Tier = "fast"
	TierDefault Tier = "default"
	TierStrong  Tier = "strong"
)

// Source is a fetched page given to the LLM as numbered context.
type Source struct {
	URL   string
	Title string
	Text  string
}

// Synthesizer runs Helix's LLMs over search results.
type Synthesizer interface {
	// Summarize returns a summary of text focused on query. With a JSON
	// schema it returns a JSON document matching it, as a string.
	Summarize(ctx context.Context, text, query string, schema json.RawMessage) (string, error)
	// ExpandQuery returns up to n alternative queries for deep search.
	ExpandQuery(ctx context.Context, query, objective string, n int) ([]string, error)
	// Describe writes a search query that would find pages like this one.
	Describe(ctx context.Context, title, text string) (string, error)
	// Answer answers query from sources. With outputSchema the answer is a
	// JSON object; otherwise a string, streamed through onDelta when set.
	Answer(ctx context.Context, req *AnswerSynthesis, onDelta func(string) error) (any, error)
	// Synthesize produces the /search output for an outputSchema, with
	// per-field grounding in the sources.
	Synthesize(ctx context.Context, req *SearchSynthesis, onDelta func(string) error) (*websearch.SearchOutput, error)
}

type AnswerSynthesis struct {
	Tier         Tier
	Query        string
	SystemPrompt string
	OutputSchema json.RawMessage
	Sources      []Source
}

type SearchSynthesis struct {
	Query        string
	Objective    string
	SystemPrompt string
	OutputSchema json.RawMessage // {"type":"text",...} or {"type":"object",...}
	Sources      []Source
}
