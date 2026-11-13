package native

import (
	"context"
	"fmt"
	"slices"
	"sort"
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
	source, err := p.loadDoc(ctx, sourceURL, fetchPolicy{timeout: defaultFetchTimeout}, websearch.TextOptions{})
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
	exclude := req.ExcludeDomains
	if req.ExcludeSourceDomain {
		// Excluded in discovery, so the engines fill the n results from
		// other sites rather than the request coming back short.
		exclude = append(slices.Clone(exclude), siteHost(sourceURL))
	}
	found, err := p.opts.Discoverer.Discover(ctx, &DiscoverRequest{
		Queries:            []string{description},
		NumResults:         p.candidatePool(n) + 1, // the source page itself is usually found, and dropped
		Category:           req.Category,
		IncludeDomains:     req.IncludeDomains,
		ExcludeDomains:     exclude,
		StartPublishedDate: req.StartPublishedDate,
		EndPublishedDate:   req.EndPublishedDate,
	})
	if err != nil {
		return nil, err
	}
	landing := landingURL(source.raw)
	candidates := slices.DeleteFunc(found.Candidates, func(c Candidate) bool {
		return c.URL == sourceURL || c.URL == landing
	})
	hits, err := p.mostSimilar(ctx, description, candidates, n)
	if err != nil {
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

// mostSimilar returns the n candidates most similar to description, best
// first, with Score set. Similarity alone orders them: the engines searched
// for an LLM's description of the page, so their order says little.
func (p *Provider) mostSimilar(ctx context.Context, description string, candidates []Candidate, n int) ([]hit, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	scores, err := p.semanticScores(ctx, description, candidates)
	if err != nil {
		return nil, err
	}
	hits := candidateHits(candidates)
	for i := range hits {
		hits[i].result.Score = &scores[i]
	}
	sort.SliceStable(hits, func(a, b int) bool { return *hits[a].result.Score > *hits[b].result.Score })
	return hits[:min(n, len(hits))], nil
}
