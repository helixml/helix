package native

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/websearch"
)

// Alternative queries the LLM adds for deep and deep-lite search.
const (
	deepExpansions     = 4
	deepLiteExpansions = 2
)

func (p *Provider) Search(ctx context.Context, req *websearch.SearchRequest) (*websearch.SearchResponse, error) {
	start := time.Now()
	resp, hits, err := p.runSearch(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(req.OutputSchema) > 0 {
		if resp.Output, err = p.opts.Synthesizer.Synthesize(ctx, searchSynthesis(req, hits), nil); err != nil {
			return nil, err
		}
	}
	resp.RequestID = system.GenerateRequestID()
	resp.SearchTime = elapsedMs(start)
	return resp, nil
}

// StreamSearch emits the results, the synthesis as text deltas (object
// schemas produce none), its grounding, then done. Errors are returned for
// the caller to report; no error event is emitted here.
func (p *Provider) StreamSearch(ctx context.Context, req *websearch.SearchRequest, emit func(websearch.SearchEvent) error) error {
	start := time.Now()
	resp, hits, err := p.runSearch(ctx, req)
	if err != nil {
		return err
	}
	if err := emit(websearch.SearchEvent{Type: websearch.SearchEventResults, Results: resp.Results}); err != nil {
		return err
	}
	if len(req.OutputSchema) > 0 {
		onDelta := func(d string) error {
			return emit(websearch.SearchEvent{Type: websearch.SearchEventTextDelta, Delta: d})
		}
		if resp.Output, err = p.opts.Synthesizer.Synthesize(ctx, searchSynthesis(req, hits), onDelta); err != nil {
			return err
		}
		if err := emit(websearch.SearchEvent{Type: websearch.SearchEventGrounding, Grounding: resp.Output.Grounding}); err != nil {
			return err
		}
	}
	return emit(websearch.SearchEvent{
		Type:       websearch.SearchEventDone,
		Output:     resp.Output,
		Warnings:   resp.Warnings,
		SearchTime: elapsedMs(start),
	})
}

// runSearch discovers and fetches everything a search needs except the
// outputSchema synthesis. It leaves RequestID and SearchTime unset.
func (p *Provider) runSearch(ctx context.Context, req *websearch.SearchRequest) (*websearch.SearchResponse, []hit, error) {
	if err := req.Validate(); err != nil {
		return nil, nil, err
	}
	typ := websearch.NormalizeSearchType(req.Type)
	deep := typ == websearch.SearchTypeDeep || typ == websearch.SearchTypeDeepLite
	switch {
	case typ != websearch.SearchTypeAuto && typ != websearch.SearchTypeLite && !deep:
		return nil, nil, websearch.FeatureDisabled("type", fmt.Sprintf("search type %q is not supported", req.Type))
	case req.Compliance != nil:
		return nil, nil, websearch.FeatureDisabled("compliance", "compliance modes are a property of the deployment")
	case deep && p.opts.Synthesizer == nil:
		return nil, nil, websearch.FeatureDisabled("type", "deep search needs an LLM, and none is configured")
	case len(req.OutputSchema) > 0 && p.opts.Synthesizer == nil:
		return nil, nil, websearch.FeatureDisabled("outputSchema", "no LLM is configured")
	}
	if req.Contents != nil {
		if err := p.checkContents(req.Contents, "contents."); err != nil {
			return nil, nil, err
		}
	}

	queries := append([]string{req.Query}, req.AdditionalQueries...)
	if deep {
		n := deepExpansions
		if typ == websearch.SearchTypeDeepLite {
			n = deepLiteExpansions
		}
		expanded, err := p.opts.Synthesizer.ExpandQuery(ctx, req.Query, req.Objective, n)
		if err != nil {
			return nil, nil, fmt.Errorf("expand query: %w", err)
		}
		queries = append(queries, expanded...)
	}
	dreq := &DiscoverRequest{
		Queries:            queries,
		NumResults:         numResults(req.NumResults),
		Category:           req.Category,
		IncludeDomains:     req.IncludeDomains,
		ExcludeDomains:     req.ExcludeDomains,
		StartPublishedDate: req.StartPublishedDate,
		EndPublishedDate:   req.EndPublishedDate,
		Moderation:         req.Moderation,
	}
	if req.UserLocation != nil {
		dreq.Country = req.UserLocation.Country
	}
	found, err := p.opts.Discoverer.Discover(ctx, dreq)
	if err != nil {
		return nil, nil, err
	}
	hits, err := candidateHits(found.Candidates)
	if err != nil {
		return nil, nil, err
	}

	contextOpt := req.Context
	if contextOpt == nil && req.Contents != nil {
		contextOpt = req.Contents.Context
	}
	render := renderConfig{textLimit: defaultSearchTextChars, query: req.Query}
	hits, pageWarnings, err := p.fetchHits(ctx, hits, pageOptions{
		contents:    req.Contents,
		needText:    contextOpt.Requested() || len(req.OutputSchema) > 0,
		includeText: req.IncludeText,
		excludeText: req.ExcludeText,
		render:      render,
	})
	if err != nil {
		return nil, nil, err
	}

	warnings := searchWarnings(req, typ, deep)
	warnings = append(warnings, found.Warnings...)
	warnings = append(warnings, pageWarnings...)
	return &websearch.SearchResponse{
		Results:            results(hits),
		ResolvedSearchType: typ,
		Warnings:           warnings,
		Context:            buildContext(hits, contextOpt, render.textLimit),
	}, hits, nil
}

// searchWarnings reports request fields that were ignored or approximated.
func searchWarnings(req *websearch.SearchRequest, typ string, deep bool) []websearch.Warning {
	w := ignoredCrawlDates(req.StartCrawlDate, req.EndCrawlDate)
	ignored := func(path, msg string) {
		w = append(w, websearch.Warning{Code: websearch.WarningIgnoredField, Path: path, Message: msg})
	}
	if loc := req.UserLocation; loc != nil && (loc.Latitude != nil || loc.Longitude != nil) {
		ignored("userLocation", "coordinates are ignored; only the country is used")
	}
	if req.Objective != "" && !deep && len(req.OutputSchema) == 0 {
		ignored("objective", fmt.Sprintf("objective is only used by deep search and outputSchema, not type %q", typ))
	}
	if req.SystemPrompt != "" && len(req.OutputSchema) == 0 {
		ignored("systemPrompt", "systemPrompt is only used with outputSchema")
	}
	for _, f := range []struct {
		path  string
		items []string
	}{{"includeText", req.IncludeText}, {"excludeText", req.ExcludeText}} {
		if len(f.items) > 0 {
			w = append(w, websearch.Warning{Code: websearch.WarningApproximatedField, Path: f.path,
				Message: f.path + " is deprecated; it filters on fetched page text, so results that could not be fetched are dropped"})
		}
	}
	return append(w, contentsWarnings(req.Contents, "contents.")...)
}

func searchSynthesis(req *websearch.SearchRequest, hits []hit) *SearchSynthesis {
	return &SearchSynthesis{
		Query:        req.Query,
		Objective:    req.Objective,
		SystemPrompt: req.SystemPrompt,
		OutputSchema: req.OutputSchema,
		Sources:      sources(hits),
	}
}
