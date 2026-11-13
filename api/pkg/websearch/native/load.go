package native

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/websearch"
)

// fetchPolicy says when a cached page may be used.
type fetchPolicy struct {
	// maxAge: nil uses a cached copy whenever there is one; -1 uses only the
	// cache; 0 always fetches; N fetches again when the copy is older than
	// N hours.
	maxAge  *int
	timeout time.Duration
}

// policyFor maps maxAgeHours, or the deprecated livecrawl, to a policy:
// never → -1 (cache only), always → 0 (always fetch), and fallback,
// preferred and auto → nil (the cached copy when there is one).
func policyFor(o *websearch.ContentsOptions) fetchPolicy {
	pol := fetchPolicy{maxAge: o.MaxAgeHours, timeout: defaultFetchTimeout}
	if o.LivecrawlTimeout > 0 {
		pol.timeout = time.Duration(o.LivecrawlTimeout) * time.Millisecond
	}
	cacheOnly, always := -1, 0
	switch o.Livecrawl {
	case websearch.LivecrawlNever:
		pol.maxAge = &cacheOnly
	case websearch.LivecrawlAlways:
		pol.maxAge = &always
	}
	return pol
}

func (pol fetchPolicy) cacheOnly() bool   { return pol.maxAge != nil && *pol.maxAge == -1 }
func (pol fetchPolicy) alwaysFetch() bool { return pol.maxAge != nil && *pol.maxAge == 0 }

func (pol fetchPolicy) fresh(page *RawPage) bool {
	if pol.maxAge == nil || *pol.maxAge < 0 {
		return true
	}
	return time.Since(page.FetchedAt) <= time.Duration(*pol.maxAge)*time.Hour
}

// textOptions are what the Extractor renders text with: the requested text
// options, or the defaults when text is not requested.
func textOptions(o *websearch.ContentsOptions) websearch.TextOptions {
	if o.Text.Requested() {
		return o.Text.TextOptions
	}
	return websearch.TextOptions{}
}

// loadDoc loads and extracts the page for a normalised URL. Failures
// specific to the page are *FetchError.
func (p *Provider) loadDoc(ctx context.Context, pageURL string, pol fetchPolicy, opts websearch.TextOptions) (*doc, error) {
	raw, source, err := p.load(ctx, pageURL, pol)
	if err != nil {
		return nil, err
	}
	page, err := p.opts.Extractor.Extract(ctx, raw, opts)
	if errors.Is(err, ErrUnsupportedDocument) {
		return nil, &FetchError{Tag: websearch.TagUnsupportedDocument, Err: err}
	}
	if err != nil {
		return nil, fmt.Errorf("extract %s: %w", raw.URL, err)
	}
	return &doc{raw: raw, page: page, source: source}, nil
}

// load returns the raw page from the cache or the web, as pol allows.
func (p *Provider) load(ctx context.Context, pageURL string, pol fetchPolicy) (*RawPage, string, error) {
	var cached *RawPage
	if !pol.alwaysFetch() {
		var err error
		if cached, err = p.opts.Cache.Get(ctx, pageURL); err != nil {
			return nil, "", fmt.Errorf("page cache: %w", err)
		}
	}
	if cached != nil && pol.fresh(cached) {
		return cached, websearch.ContentSourceCached, nil
	}
	if pol.cacheOnly() {
		return nil, "", &FetchError{Tag: websearch.TagSourceNotAvailable, Err: fmt.Errorf("%s is not cached", pageURL)}
	}
	page, err := p.fetch(ctx, pageURL, pol.timeout)
	if err != nil {
		return nil, "", err
	}
	if err := p.opts.Cache.Put(ctx, page); err != nil {
		return nil, "", fmt.Errorf("page cache: %w", err)
	}
	return page, websearch.ContentSourceCrawled, nil
}

// fetch holds one of the provider's fetch slots while fetching.
func (p *Provider) fetch(ctx context.Context, pageURL string, timeout time.Duration) (*RawPage, error) {
	select {
	case p.fetchSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-p.fetchSlots }()
	return p.opts.Fetcher.Fetch(ctx, pageURL, timeout)
}

// pageFailure returns err as a failure specific to one page, which is
// reported for that page instead of failing the request.
func pageFailure(err error) (*FetchError, bool) {
	var fe *FetchError
	if err != nil && errors.As(err, &fe) {
		return fe, true
	}
	return nil, false
}
