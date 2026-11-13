package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/websearch"
)

const (
	// defaultFetchTimeout is Exa's livecrawlTimeout default.
	defaultFetchTimeout = 10 * time.Second
	// defaultSearchTextChars caps text returned inline with /search,
	// /findSimilar and /answer, and the text each LLM source gets.
	defaultSearchTextChars = 10_000
	// defaultContentsTextChars is Exa's text.maxCharacters default on /contents.
	defaultContentsTextChars = 5_000_000
	// charsPerSentence converts the deprecated highlights.numSentences.
	charsPerSentence = 1333

	deepExpansions     = 4
	deepLiteExpansions = 2
	answerNumResults   = 8
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

// renderConfig is what rendering contents depends on besides the options.
type renderConfig struct {
	textLimit       int    // text.maxCharacters when the request sets none
	query           string // highlights query when the request sets none; "" uses the page title
	highlightScores bool   // Exa returns highlightScores on /findSimilar only
}

// pageOptions say which pages behind search hits to fetch and how to filter them.
type pageOptions struct {
	contents    *websearch.ContentsOptions // nil: no contents requested
	needText    bool                       // fetch without contents, for filters, context or synthesis
	includeText []string
	excludeText []string
	render      renderConfig
}

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
		needText:    contextEnabled(contextOpt) || len(req.OutputSchema) > 0,
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
	var w []websearch.Warning
	ignored := func(path, msg string) {
		w = append(w, websearch.Warning{Code: websearch.WarningIgnoredField, Path: path, Message: msg})
	}
	if req.StartCrawlDate != nil {
		ignored("startCrawlDate", "crawl date filters are ignored, as they are by Exa")
	}
	if req.EndCrawlDate != nil {
		ignored("endCrawlDate", "crawl date filters are ignored, as they are by Exa")
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
	if req.Contents != nil {
		w = append(w, contentsWarnings(req.Contents)...)
	}
	return w
}

// contentsWarnings reports deprecated /search contents fields that have no
// effect. /contents and /findSimilar responses have no warnings.
func contentsWarnings(o *websearch.ContentsOptions) []websearch.Warning {
	var w []websearch.Warning
	ignored := func(path string) {
		w = append(w, websearch.Warning{Code: websearch.WarningIgnoredField, Path: "contents." + path, Message: path + " is deprecated and ignored"})
	}
	if o.Highlights != nil && o.Highlights.HighlightsPerURL > 0 {
		ignored("highlights.highlightsPerUrl")
	}
	if o.Extras != nil {
		if o.Extras.RichLinks > 0 {
			ignored("extras.richLinks")
		}
		if o.Extras.CodeBlocks > 0 {
			ignored("extras.codeBlocks")
		}
	}
	return w
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

func (p *Provider) Contents(ctx context.Context, req *websearch.ContentsRequest) (*websearch.ContentsResponse, error) {
	start := time.Now()
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if req.Compliance != nil {
		return nil, websearch.FeatureDisabled("compliance", "compliance modes are a property of the deployment")
	}
	opts := &req.ContentsOptions
	if err := p.checkContents(opts, ""); err != nil {
		return nil, err
	}
	targets, byID := req.URLs, false
	if len(req.IDs) > 0 {
		targets, byID = req.IDs, true
	}
	keys := targets
	if !byID {
		keys = make([]string, len(targets))
		for i, u := range targets {
			n, err := websearch.NormalizeURL(u)
			if err != nil {
				return nil, websearch.InvalidRequest("urls[%d] is not a valid URL: %v", i, err)
			}
			keys[i] = n
		}
	}

	render := renderConfig{textLimit: defaultContentsTextChars}
	pol, textOpts := policyFor(opts), textOptions(opts)
	hits := make([]hit, len(targets))
	statuses := make([]websearch.ContentStatus, len(targets))
	err := p.forEach(ctx, len(targets), func(ctx context.Context, i int) error {
		statuses[i] = websearch.ContentStatus{ID: targets[i]}
		d, err := p.loadDoc(ctx, keys[i], byID, pol, textOpts)
		if fe, ok := pageFailure(err); ok {
			statuses[i].Status = websearch.ContentStatusError
			statuses[i].Error = &websearch.ContentError{Tag: fe.Tag, HTTPStatusCode: fe.HTTPStatusCode}
			return nil
		}
		if err != nil {
			return err
		}
		hits[i] = hit{result: newResult(d), doc: d}
		if err := p.render(ctx, &hits[i].result, d, opts, render); err != nil {
			return err
		}
		statuses[i].Status = websearch.ContentStatusSuccess
		statuses[i].Source = d.source
		return nil
	})
	if err != nil {
		return nil, err
	}
	hits = slices.DeleteFunc(hits, func(h hit) bool { return h.doc == nil })
	return &websearch.ContentsResponse{
		RequestID:  system.GenerateRequestID(),
		Results:    results(hits),
		Statuses:   statuses,
		Context:    buildContext(hits, req.Context, render.textLimit),
		SearchTime: elapsedMs(start),
	}, nil
}

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
	description, err := p.opts.Synthesizer.Describe(ctx, source.page.Title, clipChars(source.page.Text, defaultSearchTextChars))
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
	hits, _, err = p.fetchHits(ctx, hits, pageOptions{
		contents:    req.Contents,
		needText:    contextEnabled(contextOpt),
		includeText: req.IncludeText,
		excludeText: req.ExcludeText,
		render:      render,
	})
	if err != nil {
		return nil, err
	}
	return &websearch.FindSimilarResponse{
		RequestID:  system.GenerateRequestID(),
		Results:    results(hits),
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

func (p *Provider) Answer(ctx context.Context, req *websearch.AnswerRequest) (*websearch.AnswerResponse, error) {
	return p.answer(ctx, req, nil)
}

// StreamAnswer emits the answer as OpenAI-style chunks (an outputSchema
// answer as one chunk of JSON), then the citations, then the requestId.
func (p *Provider) StreamAnswer(ctx context.Context, req *websearch.AnswerRequest, emit func(websearch.AnswerEvent) error) error {
	role := "assistant"
	onDelta := func(d string) error {
		ev := websearch.AnswerEvent{Choices: []websearch.AnswerChoice{{Delta: websearch.AnswerDelta{Role: role, Content: d}}}}
		role = ""
		return emit(ev)
	}
	resp, err := p.answer(ctx, req, onDelta)
	if err != nil {
		return err
	}
	if len(req.OutputSchema) > 0 {
		b, err := json.Marshal(resp.Answer)
		if err != nil {
			return fmt.Errorf("encode answer: %w", err)
		}
		if err := onDelta(string(b)); err != nil {
			return err
		}
	}
	if err := emit(websearch.AnswerEvent{Citations: resp.Citations}); err != nil {
		return err
	}
	return emit(websearch.AnswerEvent{RequestID: resp.RequestID})
}

// answer searches, then answers from the fetched results. onDelta receives
// a string answer as it is generated.
func (p *Provider) answer(ctx context.Context, req *websearch.AnswerRequest, onDelta func(string) error) (*websearch.AnswerResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if p.opts.Synthesizer == nil {
		return nil, websearch.FeatureDisabled("answer", "no LLM is configured")
	}
	tier, searchType := answerModel(req.Model)
	_, hits, err := p.runSearch(ctx, &websearch.SearchRequest{
		Query:          req.Query,
		Type:           searchType,
		NumResults:     answerNumResults,
		UserLocation:   req.UserLocation,
		UserIdentifier: req.UserIdentifier,
		Contents:       &websearch.ContentsOptions{Text: &websearch.TextOption{Enabled: true}},
	})
	if err != nil {
		return nil, err
	}
	hits = slices.DeleteFunc(hits, func(h hit) bool { return h.doc == nil })
	answer, err := p.opts.Synthesizer.Answer(ctx, &AnswerSynthesis{
		Tier:         tier,
		Query:        req.Query,
		SystemPrompt: req.SystemPrompt,
		OutputSchema: req.OutputSchema,
		Sources:      sources(hits),
	}, onDelta)
	if err != nil {
		return nil, err
	}
	citations := results(hits)
	if !req.Text {
		for i := range citations {
			citations[i].Text = nil
		}
	}
	return &websearch.AnswerResponse{RequestID: system.GenerateRequestID(), Answer: answer, Citations: citations}, nil
}

// answerModel maps an Exa answer model to the LLM tier and search type.
func answerModel(model string) (Tier, string) {
	switch model {
	case websearch.AnswerModelExaFast:
		return TierFast, websearch.SearchTypeAuto
	case websearch.AnswerModelExaPro:
		return TierStrong, websearch.SearchTypeAuto
	case websearch.AnswerModelExaResearch:
		return TierStrong, websearch.SearchTypeDeep
	default:
		return TierDefault, websearch.SearchTypeAuto
	}
}

// checkContents rejects contents options this provider cannot honour.
func (p *Provider) checkContents(o *websearch.ContentsOptions, prefix string) error {
	switch {
	case o.SnapshotAsOf != "":
		return websearch.FeatureDisabled(prefix+"snapshotAsOf", "historical snapshots are not kept")
	case o.CrawledBefore != "":
		return websearch.FeatureDisabled(prefix+"crawledBeforeDate", "historical snapshots are not kept")
	case o.Summary != nil && p.opts.Synthesizer == nil:
		return websearch.FeatureDisabled(prefix+"summary", "no LLM is configured")
	}
	if h := o.Highlights; h != nil && h.Enabled {
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

// fetchHits fetches the pages behind hits when contents or text are needed,
// renders the requested contents onto them, and applies the text filters.
// A page that fails to fetch leaves its hit without contents.
func (p *Provider) fetchHits(ctx context.Context, hits []hit, po pageOptions) ([]hit, []websearch.Warning, error) {
	filtering := len(po.includeText) > 0 || len(po.excludeText) > 0
	if po.contents == nil && !po.needText && !filtering {
		return hits, nil, nil
	}
	opts := po.contents
	if opts == nil {
		opts = &websearch.ContentsOptions{}
	}
	pol, textOpts := policyFor(opts), textOptions(opts)
	err := p.forEach(ctx, len(hits), func(ctx context.Context, i int) error {
		d, err := p.loadDoc(ctx, hits[i].result.URL, false, pol, textOpts)
		if _, ok := pageFailure(err); ok {
			return nil
		}
		if err != nil {
			return err
		}
		hits[i].doc = d
		return p.render(ctx, &hits[i].result, d, opts, po.render)
	})
	if err != nil {
		return nil, nil, err
	}

	var warnings []websearch.Warning
	failed := 0
	for _, h := range hits {
		if h.doc == nil {
			failed++
		}
	}
	if failed > 0 {
		warnings = append(warnings, websearch.Warning{Code: websearch.WarningApproximatedField, Path: "contents",
			Message: fmt.Sprintf("%d of %d results could not be fetched and have no contents", failed, len(hits))})
	}
	if filtering {
		hits = slices.DeleteFunc(hits, func(h hit) bool {
			return h.doc == nil || !matchesText(h.doc.page.Text, po.includeText, po.excludeText)
		})
	}
	return hits, warnings, nil
}

// matchesText reports whether text contains every include phrase and no
// exclude phrase, ignoring case.
func matchesText(text string, include, exclude []string) bool {
	text = strings.ToLower(text)
	for _, s := range include {
		if !strings.Contains(text, strings.ToLower(s)) {
			return false
		}
	}
	for _, s := range exclude {
		if strings.Contains(text, strings.ToLower(s)) {
			return false
		}
	}
	return true
}

// render fills r from d: page metadata r lacks, and the contents o asks for.
func (p *Provider) render(ctx context.Context, r *websearch.Result, d *doc, o *websearch.ContentsOptions, rc renderConfig) error {
	page := d.page
	if r.Title == "" {
		r.Title = page.Title
	}
	if r.Author == nil && page.Author != "" {
		r.Author = &page.Author
	}
	if r.PublishedDate == nil {
		r.PublishedDate = page.PublishedDate
	}
	if r.Image == "" {
		r.Image = page.Image
	}
	if r.Favicon == "" {
		r.Favicon = page.Favicon
	}

	if o.Text != nil && o.Text.Enabled {
		limit := o.Text.MaxCharacters
		if limit == 0 {
			limit = rc.textLimit
		}
		text := clipChars(page.Text, limit)
		r.Text = &text
	}
	if h := o.Highlights; h != nil && h.Enabled {
		maxChars := h.MaxCharacters
		if maxChars == 0 {
			maxChars = h.NumSentences * charsPerSentence
		}
		query := h.Query
		if query == "" {
			query = rc.query
		}
		if query == "" {
			query = page.Title
		}
		highlights, scores, err := Highlights(ctx, p.opts.Ranker, query, page.Text, maxChars)
		if err != nil {
			return err
		}
		r.Highlights = highlights
		if rc.highlightScores {
			r.HighlightScores = scores
		}
	}
	if s := o.Summary; s != nil {
		summary, err := p.opts.Synthesizer.Summarize(ctx, page.Text, s.Query, s.Schema)
		if err != nil {
			return fmt.Errorf("summarize %s: %w", d.raw.URL, err)
		}
		r.Summary = &summary
	}
	if e := o.Extras; e != nil {
		r.Extras = extras(page, e)
	}
	if o.Subpages > 0 {
		subpages, err := p.subpages(ctx, d, o, rc)
		if err != nil {
			return err
		}
		r.Subpages = subpages
	}
	return nil
}

func extras(page *Page, e *websearch.ExtrasOptions) *websearch.Extras {
	var out websearch.Extras
	for _, l := range page.Links[:min(e.Links, len(page.Links))] {
		out.Links = append(out.Links, l.URL)
	}
	for _, l := range page.ImageLinks[:min(e.ImageLinks, len(page.ImageLinks))] {
		out.ImageLinks = append(out.ImageLinks, l.URL)
	}
	out.RichImageLinks = slices.Clone(page.ImageLinks[:min(e.RichImageLinks, len(page.ImageLinks))])
	if out.Links == nil && out.ImageLinks == nil && len(out.RichImageLinks) == 0 {
		return nil
	}
	return &out
}

// subpages fetches up to o.Subpages same-host pages linked from d, best
// subpageTarget match first. Subpages that fail to fetch are left out.
func (p *Provider) subpages(ctx context.Context, d *doc, o *websearch.ContentsOptions, rc renderConfig) ([]websearch.Result, error) {
	links := subpageLinks(d.raw.URL, d.page.Links, o.SubpageTarget, o.Subpages)
	sub := *o
	sub.Subpages = 0
	pol, textOpts := policyFor(o), textOptions(o)
	pages := make([]*websearch.Result, len(links))
	err := p.forEach(ctx, len(links), func(ctx context.Context, i int) error {
		sd, err := p.loadDoc(ctx, links[i], false, pol, textOpts)
		if _, ok := pageFailure(err); ok {
			return nil
		}
		if err != nil {
			return err
		}
		r := newResult(sd)
		if err := p.render(ctx, &r, sd, &sub, rc); err != nil {
			return err
		}
		pages[i] = &r
		return nil
	})
	if err != nil {
		return nil, err
	}
	var out []websearch.Result
	for _, r := range pages {
		if r != nil {
			out = append(out, *r)
		}
	}
	return out, nil
}

// subpageLinks picks up to n distinct same-host links from pageURL's links,
// ordered by how many targets each mentions in its URL or anchor, then by
// position on the page.
func subpageLinks(pageURL string, links []Link, targets []string, n int) []string {
	host := hostOf(pageURL)
	type candidate struct {
		url   string
		score int
	}
	seen := map[string]bool{pageURL: true}
	var cands []candidate
	for _, l := range links {
		u, err := websearch.NormalizeURL(l.URL)
		if err != nil || seen[u] || hostOf(u) != host {
			continue
		}
		seen[u] = true
		haystack := strings.ToLower(u + " " + l.Anchor)
		score := 0
		for _, t := range targets {
			if t != "" && strings.Contains(haystack, strings.ToLower(t)) {
				score++
			}
		}
		cands = append(cands, candidate{url: u, score: score})
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].score > cands[b].score })
	out := make([]string, 0, min(n, len(cands)))
	for _, c := range cands[:min(n, len(cands))] {
		out = append(out, c.url)
	}
	return out
}

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

func textOptions(o *websearch.ContentsOptions) websearch.TextOptions {
	if o.Text != nil && o.Text.Enabled {
		return o.Text.TextOptions
	}
	return websearch.TextOptions{}
}

// loadDoc loads and extracts the page for a normalised URL or a document
// id. Failures specific to the page are *FetchError.
func (p *Provider) loadDoc(ctx context.Context, key string, byID bool, pol fetchPolicy, opts websearch.TextOptions) (*doc, error) {
	raw, source, err := p.load(ctx, key, byID, pol)
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
func (p *Provider) load(ctx context.Context, key string, byID bool, pol fetchPolicy) (*RawPage, string, error) {
	var cached *RawPage
	var err error
	pageURL := key
	switch {
	case byID:
		if cached, err = p.opts.Cache.GetByID(ctx, key); err != nil {
			return nil, "", fmt.Errorf("page cache: %w", err)
		}
		if cached == nil {
			return nil, "", &FetchError{Tag: websearch.TagSourceNotAvailable, Err: fmt.Errorf("unknown document id %q", key)}
		}
		pageURL = cached.URL
	case !pol.alwaysFetch():
		if cached, err = p.opts.Cache.Get(ctx, key); err != nil {
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

func pageFailure(err error) (*FetchError, bool) {
	var fe *FetchError
	if err != nil && errors.As(err, &fe) {
		return fe, true
	}
	return nil, false
}

func candidateHits(candidates []Candidate) ([]hit, error) {
	hits := make([]hit, len(candidates))
	for i, c := range candidates {
		u, err := websearch.NormalizeURL(c.URL)
		if err != nil {
			return nil, fmt.Errorf("discovered URL %q: %w", c.URL, err)
		}
		hits[i].result = websearch.Result{
			ID:            websearch.DocumentID(u),
			URL:           u,
			Title:         c.Title,
			PublishedDate: c.PublishedDate,
			Image:         c.Image,
		}
	}
	return hits, nil
}

func newResult(d *doc) websearch.Result {
	return websearch.Result{ID: websearch.DocumentID(d.raw.URL), URL: d.raw.URL}
}

func results(hits []hit) []websearch.Result {
	out := make([]websearch.Result, len(hits))
	for i, h := range hits {
		out[i] = h.result
	}
	return out
}

// sources are the fetched hits as LLM context.
func sources(hits []hit) []Source {
	var out []Source
	for _, h := range hits {
		if h.doc != nil {
			out = append(out, Source{URL: h.result.URL, Title: h.result.Title, Text: clipChars(h.doc.page.Text, defaultSearchTextChars)})
		}
	}
	return out
}

func contextEnabled(o *websearch.ContextOption) bool { return o != nil && o.Enabled }

// buildContext concatenates the fetched pages' text for the deprecated
// context option.
func buildContext(hits []hit, o *websearch.ContextOption, textLimit int) string {
	if !contextEnabled(o) {
		return ""
	}
	var b strings.Builder
	for _, h := range hits {
		if h.doc == nil {
			continue
		}
		fmt.Fprintf(&b, "Title: %s\nURL: %s\n%s\n\n", h.result.Title, h.result.URL, clipChars(h.doc.page.Text, textLimit))
	}
	return clipChars(strings.TrimSpace(b.String()), o.MaxCharacters)
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
