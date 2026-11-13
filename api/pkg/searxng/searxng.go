// Package searxng is a client for the SearXNG JSON search API.
package searxng

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// SafeSearch levels.
const (
	SafeSearchOff      = 0
	SafeSearchModerate = 1
	SafeSearchStrict   = 2
)

// LanguageAuto has SearXNG detect the query's language and search in it.
const LanguageAuto = "auto"

// Time ranges.
const (
	TimeRangeDay   = "day"
	TimeRangeWeek  = "week"
	TimeRangeMonth = "month"
	TimeRangeYear  = "year"
)

// Client queries one SearXNG instance.
type Client struct {
	httpClient *http.Client
	baseURL    string
	engines    []string
}

// NewClient returns a client for the SearXNG at baseURL. engines are the
// engines general web queries run on; empty means SearXNG's defaults.
func NewClient(baseURL string, engines []string) *Client {
	return &Client{
		httpClient: http.DefaultClient,
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		engines:    engines,
	}
}

type Query struct {
	Q string
	// Categories replace the client's engines with SearXNG's engines for
	// those categories: SearXNG unions `engines` with `categories`, so
	// sending both would mix general web results into e.g. news.
	Categories []string
	Language   string // e.g. "en-US" or LanguageAuto; empty for SearXNG's default
	SafeSearch int    // SafeSearch*
	TimeRange  string // TimeRange*, or empty for no limit
	PageNo     int    // 1-based; 0 means 1
}

type Response struct {
	Results             []Result
	UnresponsiveEngines []UnresponsiveEngine
}

type Result struct {
	URL           string
	Title         string
	Content       string
	PublishedDate *time.Time
	Thumbnail     string
	ImgSrc        string
	Score         float64
	Engines       []string
}

type UnresponsiveEngine struct {
	Name   string
	Reason string
}

// Search runs one query for one page. Zero results with every queried
// engine unresponsive is an upstream failure and returns an error.
func (c *Client) Search(ctx context.Context, q Query) (*Response, error) {
	if strings.TrimSpace(q.Q) == "" {
		return nil, errors.New("searxng: empty query")
	}

	values := url.Values{}
	values.Set("q", q.Q)
	values.Set("format", "json")
	values.Set("safesearch", strconv.Itoa(q.SafeSearch))
	engines := c.engines
	if len(q.Categories) > 0 {
		engines = nil
		values.Set("categories", strings.Join(q.Categories, ","))
	}
	if len(engines) > 0 {
		values.Set("engines", strings.Join(engines, ","))
	}
	if q.Language != "" {
		values.Set("language", q.Language)
	}
	if q.TimeRange != "" {
		values.Set("time_range", q.TimeRange)
	}
	if q.PageNo > 1 {
		values.Set("pageno", strconv.Itoa(q.PageNo))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/search?"+values.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("searxng: build request: %w", err)
	}
	// SearXNG's bot detection rejects requests without a User-Agent.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; HelixBot/1.0)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("searxng: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var raw wireResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("searxng: decode response: %w", err)
	}

	out := &Response{
		Results:             make([]Result, 0, len(raw.Results)),
		UnresponsiveEngines: make([]UnresponsiveEngine, 0, len(raw.UnresponsiveEngines)),
	}
	for _, e := range raw.UnresponsiveEngines {
		if len(e) == 0 {
			return nil, errors.New("searxng: empty unresponsive_engines entry")
		}
		u := UnresponsiveEngine{Name: e[0]}
		if len(e) > 1 {
			u.Reason = e[1]
		}
		out.UnresponsiveEngines = append(out.UnresponsiveEngines, u)
	}
	for _, r := range raw.Results {
		published, err := parsePublishedDate(r.PublishedDate)
		if err != nil {
			return nil, fmt.Errorf("searxng: result %s: %w", r.URL, err)
		}
		out.Results = append(out.Results, Result{
			URL:           r.URL,
			Title:         r.Title,
			Content:       r.Content,
			PublishedDate: published,
			Thumbnail:     r.Thumbnail,
			ImgSrc:        r.ImgSrc,
			Score:         r.Score,
			Engines:       r.Engines,
		})
	}

	if len(out.Results) == 0 && allUnresponsive(engines, out.UnresponsiveEngines) {
		return nil, fmt.Errorf("searxng: no engine responded: %s", describe(out.UnresponsiveEngines))
	}
	return out, nil
}

// allUnresponsive reports whether every queried engine failed. For category
// queries the queried engines are SearXNG's to choose and not known here, so
// an empty result there is reported as empty, with UnresponsiveEngines set.
func allUnresponsive(queried []string, unresponsive []UnresponsiveEngine) bool {
	if len(queried) == 0 || len(unresponsive) == 0 {
		return false
	}
	for _, name := range queried {
		if !slices.ContainsFunc(unresponsive, func(u UnresponsiveEngine) bool { return u.Name == name }) {
			return false
		}
	}
	return true
}

func describe(engines []UnresponsiveEngine) string {
	parts := make([]string, len(engines))
	for i, e := range engines {
		parts[i] = e.Name + " (" + e.Reason + ")"
	}
	return strings.Join(parts, ", ")
}

type wireResponse struct {
	Results             []wireResult `json:"results"`
	UnresponsiveEngines [][]string   `json:"unresponsive_engines"`
}

type wireResult struct {
	URL           string   `json:"url"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	PublishedDate *string  `json:"publishedDate"`
	Thumbnail     string   `json:"thumbnail"`
	ImgSrc        string   `json:"img_src"`
	Score         float64  `json:"score"`
	Engines       []string `json:"engines"`
}

// SearXNG serialises dates with Python's isoformat: a date, or a datetime
// with optional fraction and offset. A datetime without offset is UTC.
var publishedDateLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02",
}

func parsePublishedDate(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	for _, layout := range publishedDateLayouts {
		if t, err := time.Parse(layout, *s); err == nil {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("unrecognised publishedDate %q", *s)
}
