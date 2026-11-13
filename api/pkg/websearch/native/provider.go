package native

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/helixml/helix/api/pkg/websearch"
)

const (
	// defaultFetchTimeout is Exa's livecrawlTimeout default.
	defaultFetchTimeout = 10 * time.Second
	// defaultSearchTextChars caps text returned inline with /search,
	// /findSimilar and /answer.
	defaultSearchTextChars = 10_000
	// defaultContentsTextChars is Exa's text.maxCharacters default on /contents.
	defaultContentsTextChars = 5_000_000
)

// Options are the parts a Provider is built from.
type Options struct {
	Discoverer Discoverer
	Fetcher    Fetcher
	Extractor  Extractor
	Cache      PageCache
	// Ranker is optional; without it highlights and findSimilar are disabled.
	Ranker Ranker
	// Synthesizer is optional; without it summaries, outputSchema, deep
	// search, findSimilar and answer are disabled.
	Synthesizer Synthesizer
	// FetchConcurrency bounds the pages fetched at once across all requests,
	// and the pages one request processes at once.
	FetchConcurrency int
}

// Provider is websearch.Provider on Helix's own stack.
type Provider struct {
	opts       Options
	fetchSlots chan struct{}
}

var _ websearch.Provider = (*Provider)(nil)

func New(opts Options) (*Provider, error) {
	switch {
	case opts.Discoverer == nil:
		return nil, errors.New("native websearch: Discoverer is required")
	case opts.Fetcher == nil:
		return nil, errors.New("native websearch: Fetcher is required")
	case opts.Extractor == nil:
		return nil, errors.New("native websearch: Extractor is required")
	case opts.Cache == nil:
		return nil, errors.New("native websearch: Cache is required")
	case opts.FetchConcurrency < 1:
		return nil, errors.New("native websearch: FetchConcurrency must be at least 1")
	}
	return &Provider{opts: opts, fetchSlots: make(chan struct{}, opts.FetchConcurrency)}, nil
}

// hit is a result and the page behind it; doc is nil when the page was not
// fetched or could not be.
type hit struct {
	result websearch.Result
	doc    *doc
}

type doc struct {
	raw    *RawPage
	page   *Page
	source string // websearch.ContentSource*
}

// checkContents rejects contents options this provider cannot honour.
func (p *Provider) checkContents(o *websearch.ContentsOptions, prefix string) error {
	switch {
	case o.SnapshotAsOf != "":
		return websearch.FeatureDisabled(prefix+"snapshotAsOf", "historical snapshots are not kept")
	case o.CrawledBefore != "":
		return websearch.FeatureDisabled(prefix+"crawledBeforeDate", "historical snapshots are not kept")
	case o.Summary.Requested() && p.opts.Synthesizer == nil:
		return websearch.FeatureDisabled(prefix+"summary", "no LLM is configured")
	}
	if h := o.Highlights; h.Requested() {
		switch {
		case h.Dynamic:
			return websearch.FeatureDisabled(prefix+"highlights.dynamic", "beta highlights are not supported")
		case h.Verbosity != "":
			return websearch.FeatureDisabled(prefix+"highlights.verbosity", "beta highlights are not supported")
		case p.opts.Ranker == nil:
			return websearch.FeatureDisabled(prefix+"highlights", "no ranking model is configured")
		}
	}
	return nil
}

// forEach runs fn for 0..n-1, at most FetchConcurrency at once, stopping
// at the first error.
func (p *Provider) forEach(ctx context.Context, n int, fn func(ctx context.Context, i int) error) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.opts.FetchConcurrency)
	for i := range n {
		g.Go(func() error { return fn(ctx, i) })
	}
	return g.Wait()
}

func candidateHits(candidates []Candidate) ([]hit, error) {
	hits := make([]hit, len(candidates))
	for i, c := range candidates {
		u, err := websearch.NormalizeURL(c.URL)
		if err != nil {
			return nil, fmt.Errorf("discovered URL %q: %w", c.URL, err)
		}
		hits[i].result = websearch.Result{
			ID:            u,
			URL:           u,
			Title:         c.Title,
			PublishedDate: c.PublishedDate,
			Image:         c.Image,
		}
	}
	return hits, nil
}

func newResult(d *doc) websearch.Result {
	return websearch.Result{ID: d.raw.URL, URL: d.raw.URL}
}

func results(hits []hit) []websearch.Result {
	out := make([]websearch.Result, len(hits))
	for i, h := range hits {
		out[i] = h.result
	}
	return out
}

// sources are the fetched hits as LLM context. The Synthesizer owns the
// prompt budget and truncates them.
func sources(hits []hit) []Source {
	var out []Source
	for _, h := range hits {
		if h.doc != nil {
			out = append(out, Source{URL: h.result.URL, Title: h.result.Title, Text: h.doc.page.Text})
		}
	}
	return out
}

// ignoredCrawlDates warns about crawl date filters, which Exa ignores too.
func ignoredCrawlDates(start, end *time.Time) []websearch.Warning {
	var w []websearch.Warning
	for _, f := range []struct {
		path string
		set  bool
	}{{"startCrawlDate", start != nil}, {"endCrawlDate", end != nil}} {
		if f.set {
			w = append(w, websearch.Warning{Code: websearch.WarningIgnoredField, Path: f.path,
				Message: "crawl date filters are ignored, as they are by Exa"})
		}
	}
	return w
}

func partialResults(path, msg string) websearch.Warning {
	return websearch.Warning{Code: websearch.WarningPartialResults, Path: path, Message: msg}
}

// clipChars returns the first n characters of s; n <= 0 means no limit.
func clipChars(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}

func numResults(n int) int {
	if n == 0 {
		return websearch.DefaultNumResults
	}
	return n
}

// hostOf is the host of a normalised URL.
func hostOf(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// siteHost is the host without a leading "www.", for same-site checks.
func siteHost(u string) string {
	return strings.TrimPrefix(hostOf(u), "www.")
}

func elapsedMs(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000
}
