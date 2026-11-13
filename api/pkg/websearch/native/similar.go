package native

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/websearch"
)

// FindSimilar describes the page with the LLM, searches for the
// description, and scores each result against it.
func (p *Provider) FindSimilar(ctx context.Context, req *websearch.FindSimilarRequest) (*websearch.FindSimilarResponse, error) {
	start := time.Now()
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if p.opts.Synthesizer == nil || p.opts.Ranker == nil {
		return nil, websearch.FeatureDisabled("findSimilar", "it needs an LLM and a ranking model")
	}
	if req.Contents != nil {
		if err := p.checkContents(req.Contents, "contents."); err != nil {
			return nil, err
		}
	}
	sourceURL, err := websearch.NormalizeURL(req.URL)
	if err != nil {
		return nil, websearch.InvalidRequest("url is not a valid URL: %v", err)
	}
	source, err := p.loadDoc(ctx, sourceURL, false, fetchPolicy{timeout: defaultFetchTimeout}, websearch.TextOptions{})
	if fe, ok := pageFailure(err); ok {
		return nil, websearch.InvalidRequest("url could not be fetched: %s", fe.Tag)
	}
	if err != nil {
		return nil, err
	}
	description, err := p.opts.Synthesizer.Describe(ctx, source.page.Title, source.page.Text)
	if err != nil {
		return nil, fmt.Errorf("describe source page: %w", err)
	}

	n := numResults(req.NumResults)
	found, err := p.opts.Discoverer.Discover(ctx, &DiscoverRequest{
		Queries:            []string{description},
		NumResults:         n + 1, // the source page itself is usually found, and dropped
		Category:           req.Category,
		IncludeDomains:     req.IncludeDomains,
		ExcludeDomains:     req.ExcludeDomains,
		StartPublishedDate: req.StartPublishedDate,
		EndPublishedDate:   req.EndPublishedDate,
	})
	if err != nil {
		return nil, err
	}
	sourceHost := siteHost(sourceURL)
	var candidates []Candidate
	for _, c := range found.Candidates {
		u, err := websearch.NormalizeURL(c.URL)
		if err != nil {
			return nil, fmt.Errorf("discovered URL %q: %w", c.URL, err)
		}
		if u == sourceURL || (req.ExcludeSourceDomain && siteHost(u) == sourceHost) {
			continue
		}
		candidates = append(candidates, c)
	}
	if len(candidates) > n {
		candidates = candidates[:n]
	}
	hits, err := candidateHits(candidates)
	if err != nil {
		return nil, err
	}
	if err := p.score(ctx, description, candidates, hits); err != nil {
		return nil, err
	}

	var contextOpt *websearch.ContextOption
	if req.Contents != nil {
		contextOpt = req.Contents.Context
	}
	render := renderConfig{textLimit: defaultSearchTextChars, query: description, highlightScores: true}
	hits, pageWarnings, err := p.fetchHits(ctx, hits, pageOptions{
		contents:    req.Contents,
		needText:    contextOpt.Requested(),
		includeText: req.IncludeText,
		excludeText: req.ExcludeText,
		render:      render,
	})
	if err != nil {
		return nil, err
	}
	warnings := ignoredCrawlDates(req.StartCrawlDate, req.EndCrawlDate)
	warnings = append(warnings, contentsWarnings(req.Contents, "contents.")...)
	warnings = append(warnings, found.Warnings...)
	warnings = append(warnings, pageWarnings...)
	return &websearch.FindSimilarResponse{
		RequestID:  system.GenerateRequestID(),
		Results:    results(hits),
		Warnings:   warnings,
		Context:    buildContext(hits, contextOpt, render.textLimit),
		SearchTime: elapsedMs(start),
	}, nil
}

// score sets each hit's Score from its title and snippet, best first.
func (p *Provider) score(ctx context.Context, query string, candidates []Candidate, hits []hit) error {
	if len(hits) == 0 {
		return nil
	}
	passages := make([]string, len(candidates))
	for i, c := range candidates {
		passages[i] = strings.TrimSpace(c.Title + "\n" + c.Snippet)
	}
	scores, err := p.opts.Ranker.Rank(ctx, query, passages)
	if err != nil {
		return fmt.Errorf("score similar pages: %w", err)
	}
	if len(scores) != len(hits) {
		return fmt.Errorf("score similar pages: ranker returned %d scores for %d pages", len(scores), len(hits))
	}
	for i := range hits {
		hits[i].result.Score = &scores[i]
	}
	sort.SliceStable(hits, func(a, b int) bool { return *hits[a].result.Score > *hits[b].result.Score })
	return nil
}
