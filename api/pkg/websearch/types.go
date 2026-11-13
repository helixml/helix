// Package websearch is Helix's web search, contents and answer service. Its
// request and response types are the Exa API wire format (OpenAPI 2.0.0), so
// Exa's own SDKs work against Helix with only the base URL changed, and a
// deployment can serve them from Helix's own stack or from Exa.
package websearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Search types. The enum is open upstream; these are the values Helix
// recognises. Legacy values are normalised by NormalizeSearchType.
const (
	SearchTypeAuto     = "auto"
	SearchTypeLite     = "lite"
	SearchTypeDeepLite = "deep-lite"
	SearchTypeDeep     = "deep"
)

// Categories.
const (
	CategoryCompany         = "company"
	CategoryPublication     = "publication"
	CategoryNews            = "news"
	CategoryPersonalSite    = "personal site"
	CategoryFinancialReport = "financial report"
	CategoryPeople          = "people"
)

// SearchRequest is the body of POST /search.
type SearchRequest struct {
	Query              string           `json:"query"`
	Type               string           `json:"type,omitempty"`
	AdditionalQueries  []string         `json:"additionalQueries,omitempty"`
	NumResults         int              `json:"numResults,omitempty"`
	Category           string           `json:"category,omitempty"`
	IncludeDomains     []string         `json:"includeDomains,omitempty"`
	ExcludeDomains     []string         `json:"excludeDomains,omitempty"`
	StartPublishedDate *time.Time       `json:"startPublishedDate,omitempty"`
	EndPublishedDate   *time.Time       `json:"endPublishedDate,omitempty"`
	StartCrawlDate     *time.Time       `json:"startCrawlDate,omitempty"`
	EndCrawlDate       *time.Time       `json:"endCrawlDate,omitempty"`
	IncludeText        []string         `json:"includeText,omitempty"`
	ExcludeText        []string         `json:"excludeText,omitempty"`
	UserLocation       *UserLocation    `json:"userLocation,omitempty"`
	Moderation         bool             `json:"moderation,omitempty"`
	Contents           *ContentsOptions `json:"contents,omitempty"`
	OutputSchema       json.RawMessage  `json:"outputSchema,omitempty"`
	Objective          string           `json:"objective,omitempty"`
	SystemPrompt       string           `json:"systemPrompt,omitempty"`
	Stream             bool             `json:"stream,omitempty"`
	Compliance         *string          `json:"compliance,omitempty"`
	Context            *ContextOption   `json:"context,omitempty"`
	UserIdentifier     *string          `json:"userIdentifier,omitempty"`
}

// UserLocation is either an ISO 3166-1 alpha-2 string or an object.
type UserLocation struct {
	Country   string   `json:"country,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

func (l *UserLocation) UnmarshalJSON(b []byte) error {
	var country string
	if err := json.Unmarshal(b, &country); err == nil {
		l.Country = country
		return nil
	}
	type plain UserLocation
	return json.Unmarshal(b, (*plain)(l))
}

// ContentsOptions selects what to return for each page. It is the `contents`
// object of /search and /findSimilar, and is inlined into /contents.
type ContentsOptions struct {
	Text             *TextOption       `json:"text,omitempty"`
	Highlights       *HighlightsOption `json:"highlights,omitempty"`
	Summary          *SummaryOption    `json:"summary,omitempty"`
	Extras           *ExtrasOptions    `json:"extras,omitempty"`
	Livecrawl        string            `json:"livecrawl,omitempty"`
	LivecrawlTimeout int               `json:"livecrawlTimeout,omitempty"`
	MaxAgeHours      *int              `json:"maxAgeHours,omitempty"`
	SnapshotAsOf     string            `json:"snapshotAsOf,omitempty"`
	CrawledBefore    string            `json:"crawledBeforeDate,omitempty"`
	Subpages         int               `json:"subpages,omitempty"`
	SubpageTarget    StringList        `json:"subpageTarget,omitempty"`
	Context          *ContextOption    `json:"context,omitempty"`
}

// TextOption is `true` or a TextOptions object.
type TextOption struct {
	Enabled bool
	TextOptions
}

type TextOptions struct {
	MaxCharacters   int      `json:"maxCharacters,omitempty"`
	IncludeHTMLTags bool     `json:"includeHtmlTags,omitempty"`
	LinkStyle       string   `json:"linkStyle,omitempty"` // LinkStyle*
	Verbosity       string   `json:"verbosity,omitempty"` // Verbosity*
	IncludeSections []string `json:"includeSections,omitempty"`
	ExcludeSections []string `json:"excludeSections,omitempty"`
}

func (o *TextOption) UnmarshalJSON(b []byte) error {
	return unmarshalBoolOrObject(b, &o.Enabled, &o.TextOptions)
}

func (o TextOption) MarshalJSON() ([]byte, error) {
	return marshalBoolOrObject(o.Enabled, o.TextOptions)
}

// Requested reports whether text is asked for: set and not `false`.
func (o *TextOption) Requested() bool { return o != nil && o.Enabled }

// HighlightsOption is `true` or a HighlightsOptions object.
type HighlightsOption struct {
	Enabled bool
	HighlightsOptions
}

type HighlightsOptions struct {
	Query            string `json:"query,omitempty"`
	MaxCharacters    int    `json:"maxCharacters,omitempty"`
	NumSentences     int    `json:"numSentences,omitempty"`     // deprecated: ~1333 chars per sentence
	HighlightsPerURL int    `json:"highlightsPerUrl,omitempty"` // deprecated: ignored
	Verbosity        string `json:"verbosity,omitempty"`        // beta
	Dynamic          bool   `json:"dynamic,omitempty"`          // beta
}

func (o *HighlightsOption) UnmarshalJSON(b []byte) error {
	return unmarshalBoolOrObject(b, &o.Enabled, &o.HighlightsOptions)
}

func (o HighlightsOption) MarshalJSON() ([]byte, error) {
	return marshalBoolOrObject(o.Enabled, o.HighlightsOptions)
}

// Requested reports whether highlights are asked for: set and not `false`.
func (o *HighlightsOption) Requested() bool { return o != nil && o.Enabled }

// ContextOption is `true` or {maxCharacters}. Deprecated upstream.
type ContextOption struct {
	Enabled bool
	ContextOptions
}

type ContextOptions struct {
	MaxCharacters int `json:"maxCharacters,omitempty"`
}

func (o *ContextOption) UnmarshalJSON(b []byte) error {
	return unmarshalBoolOrObject(b, &o.Enabled, &o.ContextOptions)
}

func (o ContextOption) MarshalJSON() ([]byte, error) {
	return marshalBoolOrObject(o.Enabled, o.ContextOptions)
}

// Requested reports whether context is asked for: set and not `false`.
func (o *ContextOption) Requested() bool { return o != nil && o.Enabled }

// SummaryOption is `true` or a SummaryOptions object.
type SummaryOption struct {
	Enabled bool
	SummaryOptions
}

func (o *SummaryOption) UnmarshalJSON(b []byte) error {
	return unmarshalBoolOrObject(b, &o.Enabled, &o.SummaryOptions)
}

func (o SummaryOption) MarshalJSON() ([]byte, error) {
	return marshalBoolOrObject(o.Enabled, o.SummaryOptions)
}

// Requested reports whether a summary is asked for: set and not `false`.
func (o *SummaryOption) Requested() bool { return o != nil && o.Enabled }

type SummaryOptions struct {
	Query  string          `json:"query,omitempty"`
	Schema json.RawMessage `json:"schema,omitempty"`
}

type ExtrasOptions struct {
	Links          int `json:"links,omitempty"`
	ImageLinks     int `json:"imageLinks,omitempty"`
	RichImageLinks int `json:"richImageLinks,omitempty"`
	RichLinks      int `json:"richLinks,omitempty"`  // deprecated
	CodeBlocks     int `json:"codeBlocks,omitempty"` // deprecated
}

// StringList is a string or an array of strings.
type StringList []string

func (s *StringList) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*s = StringList{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return fmt.Errorf("expected a string or an array of strings")
	}
	*s = many
	return nil
}

// ContentsRequest is the body of POST /contents: exactly one of IDs or URLs,
// plus inline ContentsOptions.
type ContentsRequest struct {
	IDs  []string `json:"ids,omitempty"`
	URLs []string `json:"urls,omitempty"`
	ContentsOptions
	Compliance     *string `json:"compliance,omitempty"`
	UserIdentifier *string `json:"userIdentifier,omitempty"`
}

// FindSimilarRequest is the body of POST /findSimilar (deprecated upstream).
type FindSimilarRequest struct {
	URL                 string           `json:"url"`
	NumResults          int              `json:"numResults,omitempty"`
	IncludeDomains      []string         `json:"includeDomains,omitempty"`
	ExcludeDomains      []string         `json:"excludeDomains,omitempty"`
	StartPublishedDate  *time.Time       `json:"startPublishedDate,omitempty"`
	EndPublishedDate    *time.Time       `json:"endPublishedDate,omitempty"`
	StartCrawlDate      *time.Time       `json:"startCrawlDate,omitempty"`
	EndCrawlDate        *time.Time       `json:"endCrawlDate,omitempty"`
	IncludeText         []string         `json:"includeText,omitempty"`
	ExcludeText         []string         `json:"excludeText,omitempty"`
	Category            string           `json:"category,omitempty"`
	ExcludeSourceDomain bool             `json:"excludeSourceDomain,omitempty"`
	Contents            *ContentsOptions `json:"contents,omitempty"`
	UserIdentifier      *string          `json:"userIdentifier,omitempty"`
}

// Answer models.
const (
	AnswerModelExa         = "exa"
	AnswerModelExaPro      = "exa-pro"
	AnswerModelExaResearch = "exa-research"
	AnswerModelExaFast     = "exa-fast"
)

// AnswerRequest is the body of POST /answer.
type AnswerRequest struct {
	Query          string          `json:"query"`
	Stream         bool            `json:"stream,omitempty"`
	Text           bool            `json:"text,omitempty"`
	Model          string          `json:"model,omitempty"`
	SystemPrompt   string          `json:"systemPrompt,omitempty"`
	UserLocation   *UserLocation   `json:"userLocation,omitempty"`
	OutputSchema   json.RawMessage `json:"outputSchema,omitempty"`
	UserIdentifier *string         `json:"userIdentifier,omitempty"`
}

// Result is one page in /search, /contents and /findSimilar responses, and a
// citation in /answer.
type Result struct {
	ID              string     `json:"id,omitempty"`
	URL             string     `json:"url"`
	Title           string     `json:"title"`
	PublishedDate   *time.Time `json:"publishedDate,omitempty"`
	Author          *string    `json:"author,omitempty"`
	Image           string     `json:"image,omitempty"`
	Favicon         string     `json:"favicon,omitempty"`
	Text            *string    `json:"text,omitempty"`
	Highlights      []string   `json:"highlights,omitempty"`
	HighlightScores []float64  `json:"highlightScores,omitempty"` // findSimilar only
	Summary         *string    `json:"summary,omitempty"`
	Subpages        []Result   `json:"subpages,omitempty"`
	Extras          *Extras    `json:"extras,omitempty"`
	Score           *float64   `json:"score,omitempty"` // findSimilar only
	// Entities are Exa's structured company/person records; only the exa
	// provider returns them.
	Entities []json.RawMessage `json:"entities,omitempty"`
}

type Extras struct {
	Links          []string    `json:"links,omitempty"`
	ImageLinks     []string    `json:"imageLinks,omitempty"`
	RichImageLinks []ImageLink `json:"richImageLinks,omitempty"`
}

type ImageLink struct {
	URL string `json:"url"`
	Alt string `json:"alt,omitempty"`
}

// Warning is developer-facing: a field Helix ignored or approximated. Path
// names the request field in dotted form ("contents.text",
// "additionalQueries[1]"), or is empty for the request as a whole; on the
// wire it is a JSON Pointer, as Exa sends.
type Warning struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (w Warning) MarshalJSON() ([]byte, error) {
	type wire Warning
	w.Path = jsonPointer(w.Path)
	return json.Marshal(wire(w))
}

// jsonPointer turns a dotted field path into a JSON Pointer. Paths that are
// already pointers (from Exa) pass through.
func jsonPointer(path string) string {
	if path == "" || strings.HasPrefix(path, "/") {
		return path
	}
	path = strings.NewReplacer("[", ".", "]", "").Replace(path)
	return "/" + strings.ReplaceAll(path, ".", "/")
}

// Warning codes.
const (
	WarningIgnoredField      = "IGNORED_FIELD"
	WarningApproximatedField = "APPROXIMATED_FIELD"
	// WarningPartialResults: some queries, pages or engines failed and the
	// results are what the rest returned.
	WarningPartialResults = "PARTIAL_RESULTS"
)

// Notice is model-facing context about the results (exa provider only).
type Notice struct {
	Code    string          `json:"code"`
	Path    string          `json:"path,omitempty"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitempty"`
}

// CostDollars is what a request cost the caller in Helix credits. Contents
// itemises the content charges; the search charge appears only in Total,
// which Exa allows ("some search types report their cost only in total"),
// since Exa's neural/keyword split does not describe Helix's price table.
type CostDollars struct {
	Total    float64       `json:"total"`
	Contents *ContentsCost `json:"contents,omitempty"`
}

type ContentsCost struct {
	Text       *float64 `json:"text,omitempty"`
	Highlights *float64 `json:"highlights,omitempty"`
	Summary    *float64 `json:"summary,omitempty"`
}

type SearchResponse struct {
	RequestID          string        `json:"requestId"`
	Results            []Result      `json:"results"`
	ResolvedSearchType string        `json:"resolvedSearchType"`
	SearchTime         float64       `json:"searchTime"`
	Warnings           []Warning     `json:"warnings,omitempty"`
	Notices            []Notice      `json:"notices,omitempty"`
	Context            string        `json:"context,omitempty"`
	Output             *SearchOutput `json:"output,omitempty"`
	CostDollars        *CostDollars  `json:"costDollars,omitempty"`
}

// SearchOutput is the synthesis produced when outputSchema is set. Content is
// a string for {"type":"text"} schemas, otherwise an object.
type SearchOutput struct {
	Content   any         `json:"content"`
	Grounding []Grounding `json:"grounding"`
}

type Grounding struct {
	Field      string     `json:"field"`
	Citations  []Citation `json:"citations"`
	Confidence string     `json:"confidence"` // low | medium | high
}

type Citation struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

type ContentsResponse struct {
	RequestID   string          `json:"requestId"`
	Results     []Result        `json:"results"`
	Statuses    []ContentStatus `json:"statuses"`
	SearchTime  float64         `json:"searchTime"`
	Warnings    []Warning       `json:"warnings,omitempty"`
	Context     string          `json:"context,omitempty"`
	CostDollars *CostDollars    `json:"costDollars,omitempty"`
}

// ContentStatus reports the outcome for one requested URL or id.
type ContentStatus struct {
	ID     string        `json:"id"`
	Status string        `json:"status"`           // success | error
	Source string        `json:"source,omitempty"` // cached | crawled
	Error  *ContentError `json:"error,omitempty"`
}

type ContentError struct {
	Tag            string `json:"tag"`
	HTTPStatusCode int    `json:"httpStatusCode,omitempty"`
}

// Content status values.
const (
	ContentStatusSuccess = "success"
	ContentStatusError   = "error"
	ContentSourceCached  = "cached"
	ContentSourceCrawled = "crawled"
)

// Content error tags.
const (
	TagCrawlNotFound       = "CRAWL_NOT_FOUND"
	TagCrawlTimeout        = "CRAWL_TIMEOUT"
	TagCrawlHTTPError      = "CRAWL_HTTP_ERROR"
	TagSourceNotAvailable  = "SOURCE_NOT_AVAILABLE"
	TagBlockedDestination  = "BLOCKED_DESTINATION"
	TagUnsupportedDocument = "UNSUPPORTED_DOCUMENT"
)

type FindSimilarResponse struct {
	RequestID   string       `json:"requestId"`
	Results     []Result     `json:"results"`
	SearchTime  float64      `json:"searchTime"`
	Warnings    []Warning    `json:"warnings,omitempty"`
	Context     string       `json:"context,omitempty"`
	CostDollars *CostDollars `json:"costDollars,omitempty"`
}

type AnswerResponse struct {
	RequestID   string       `json:"requestId"`
	Answer      any          `json:"answer"` // string, or object when outputSchema is set
	Citations   []Result     `json:"citations"`
	CostDollars *CostDollars `json:"costDollars,omitempty"`
}

// SearchEvent is one SSE chunk of a streamed /search (outputSchema + stream).
type SearchEvent struct {
	RequestID   string         `json:"requestId,omitempty"`
	Type        string         `json:"type"` // text-delta | grounding | results | stream-reset | done | error
	Delta       *string        `json:"delta,omitempty"` // set on text-delta, even when empty
	Choices     []StreamChoice `json:"choices,omitempty"`
	Grounding   []Grounding    `json:"grounding,omitempty"`
	Citations   []Citation     `json:"citations,omitempty"`
	Results     []Result       `json:"results,omitempty"`
	Warnings    []Warning      `json:"warnings,omitempty"`
	Notices     []Notice       `json:"notices,omitempty"`
	StreamReset bool           `json:"streamReset,omitempty"`
	Output      *SearchOutput  `json:"output,omitempty"`
	SearchTime  float64        `json:"searchTime,omitempty"`
	CostDollars *CostDollars   `json:"costDollars,omitempty"`
	Error       *EventError    `json:"error,omitempty"`
}

// Search event types.
const (
	SearchEventTextDelta = "text-delta"
	SearchEventGrounding = "grounding"
	SearchEventResults   = "results"
	SearchEventReset     = "stream-reset"
	SearchEventDone      = "done"
	SearchEventError     = "error"
)

type EventError struct {
	Message string `json:"message"`
}

// AnswerEvent is one SSE chunk of a streamed /answer: a text delta in
// OpenAI chat-chunk shape, the citations, or the final cost.
type AnswerEvent struct {
	Choices     []StreamChoice `json:"choices,omitempty"`
	Citations   []Result       `json:"citations,omitempty"`
	CostDollars *CostDollars   `json:"costDollars,omitempty"`
	RequestID   string         `json:"requestId,omitempty"`
}

// AnswerErrorEvent is the event a streamed /answer ends with when it fails
// after streaming started: {"tag":"ERROR","payload":{"error":{...}}}.
type AnswerErrorEvent struct {
	Tag     string             `json:"tag"` // AnswerErrorTag
	Payload AnswerErrorPayload `json:"payload"`
}

const AnswerErrorTag = "ERROR"

type AnswerErrorPayload struct {
	Error     AnswerErrorDetail `json:"error"`
	RequestID string            `json:"requestId,omitempty"`
}

type AnswerErrorDetail struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        StreamDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type StreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content"`
}

func unmarshalBoolOrObject(b []byte, enabled *bool, obj any) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("true")) || bytes.Equal(b, []byte("false")) {
		return json.Unmarshal(b, enabled)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(obj); err != nil {
		return err
	}
	*enabled = true
	return nil
}

func marshalBoolOrObject(enabled bool, obj any) ([]byte, error) {
	if !enabled || reflect.ValueOf(obj).IsZero() {
		return json.Marshal(enabled)
	}
	return json.Marshal(obj)
}
