package native

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/websearch"
)

// env wires mock components. The fake web, cache and extractor are opt-in
// so a test that does not set them up fails if the provider touches them.
type env struct {
	disc  *MockDiscoverer
	fetch *MockFetcher
	ext   *MockExtractor
	cache *MockPageCache
	rank  *MockRanker
	synth *MockSynthesizer
	opts  Options

	mu      sync.Mutex
	cached  map[string]*RawPage
	fetched []string // URLs fetched, in order
	puts    []string
	bodies  map[string]string // page text overrides
	pages   map[string]*Page  // extracted page overrides
}

func newEnv(t *testing.T) *env {
	ctrl := gomock.NewController(t)
	e := &env{
		disc:   NewMockDiscoverer(ctrl),
		fetch:  NewMockFetcher(ctrl),
		ext:    NewMockExtractor(ctrl),
		cache:  NewMockPageCache(ctrl),
		rank:   NewMockRanker(ctrl),
		synth:  NewMockSynthesizer(ctrl),
		cached: map[string]*RawPage{},
		bodies: map[string]string{},
		pages:  map[string]*Page{},
	}
	e.opts = Options{Discoverer: e.disc, Fetcher: e.fetch, Extractor: e.ext, Cache: e.cache,
		Ranker: e.rank, Synthesizer: e.synth, FetchConcurrency: 4}
	return e
}

func (e *env) provider(t *testing.T) *Provider {
	t.Helper()
	p, err := New(e.opts)
	require.NoError(t, err)
	return p
}

func bodyOf(u string) string { return "text of " + u }

// web serves every URL except those in fail, and records fetches.
func (e *env) web(fail map[string]error) {
	e.fetch.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, u string, _ time.Duration) (*RawPage, error) {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.fetched = append(e.fetched, u)
			if err, ok := fail[u]; ok {
				return nil, err
			}
			body, ok := e.bodies[u]
			if !ok {
				body = bodyOf(u)
			}
			return &RawPage{URL: u, FinalURL: u, StatusCode: 200, ContentType: "text/html", Body: []byte(body), FetchedAt: time.Now()}, nil
		}).AnyTimes()
}

// memCache backs the cache mock with a map.
func (e *env) memCache() {
	e.cache.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u string) (*RawPage, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.cached[u], nil
	}).AnyTimes()
	e.cache.EXPECT().GetByID(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id string) (*RawPage, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		for u, p := range e.cached {
			if websearch.DocumentID(u) == id {
				return p, nil
			}
		}
		return nil, nil
	}).AnyTimes()
	e.cache.EXPECT().Put(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, p *RawPage) error {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.cached[p.URL] = p
		e.puts = append(e.puts, p.URL)
		return nil
	}).AnyTimes()
}

func (e *env) cachePage(u string, age time.Duration) {
	e.cached[u] = &RawPage{URL: u, StatusCode: 200, ContentType: "text/html", Body: []byte("cached " + bodyOf(u)), FetchedAt: time.Now().Add(-age)}
}

// extract turns a raw page into Page{Title: "Page <url>", Text: body}.
func (e *env) extract() {
	e.ext.EXPECT().Extract(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, raw *RawPage, _ websearch.TextOptions) (*Page, error) {
			e.mu.Lock()
			defer e.mu.Unlock()
			if p, ok := e.pages[raw.URL]; ok {
				cp := *p
				if cp.Text == "" {
					cp.Text = string(raw.Body)
				}
				return &cp, nil
			}
			return &Page{Title: "Page " + raw.URL, Text: string(raw.Body), Author: "Ann", Favicon: raw.URL + "favicon.ico"}, nil
		}).AnyTimes()
}

// pipeline enables the fake web, cache and extractor.
func (e *env) pipeline(fail map[string]error) {
	e.web(fail)
	e.memCache()
	e.extract()
}

// discover returns candidates and records the request.
func (e *env) discover(cands ...Candidate) *DiscoverRequest {
	got := &DiscoverRequest{}
	e.disc.EXPECT().Discover(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, r *DiscoverRequest) (*DiscoverResult, error) {
		*got = *r
		return &DiscoverResult{Candidates: cands}, nil
	})
	return got
}

func cands(urls ...string) []Candidate {
	out := make([]Candidate, len(urls))
	for i, u := range urls {
		out[i] = Candidate{URL: u, Title: "Candidate " + u, Snippet: "snippet " + u}
	}
	return out
}

func requireTag(t *testing.T, err error, tag string) {
	t.Helper()
	require.Error(t, err)
	e := websearch.AsError(err)
	assert.Equal(t, http.StatusBadRequest, e.Status, e.Message)
	assert.Equal(t, tag, e.Tag, e.Message)
}

func ptr[T any](v T) *T { return &v }

func resultTexts(rs []websearch.Result) []string {
	var out []string
	for _, r := range rs {
		if r.Text == nil {
			out = append(out, "<nil>")
		} else {
			out = append(out, *r.Text)
		}
	}
	return out
}

func resultURLs(rs []websearch.Result) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.URL)
	}
	return out
}

func TestNew(t *testing.T) {
	e := newEnv(t)
	for name, mutate := range map[string]func(*Options){
		"no discoverer":    func(o *Options) { o.Discoverer = nil },
		"no fetcher":       func(o *Options) { o.Fetcher = nil },
		"no extractor":     func(o *Options) { o.Extractor = nil },
		"no cache":         func(o *Options) { o.Cache = nil },
		"zero concurrency": func(o *Options) { o.FetchConcurrency = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			opts := e.opts
			mutate(&opts)
			_, err := New(opts)
			require.Error(t, err)
		})
	}
	opts := e.opts
	opts.Ranker, opts.Synthesizer = nil, nil
	_, err := New(opts)
	require.NoError(t, err, "ranker and synthesizer are optional")
}

func TestSearchWithoutContents(t *testing.T) {
	e := newEnv(t)
	published := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	got := e.discover(
		Candidate{URL: "HTTPS://Example.com:443/a#frag", Title: "A", PublishedDate: &published, Image: "https://example.com/a.png"},
		Candidate{URL: "https://b.com", Title: "B"},
	)
	resp, err := e.provider(t).Search(context.Background(), &websearch.SearchRequest{
		Query: "q", Type: "fast", Category: "news", IncludeDomains: []string{"example.com"},
		UserLocation: &websearch.UserLocation{Country: "DE"}, Moderation: true,
	})
	require.NoError(t, err)

	assert.Equal(t, &DiscoverRequest{Queries: []string{"q"}, NumResults: 10, Category: "news",
		IncludeDomains: []string{"example.com"}, Country: "DE", Moderation: true}, got)
	assert.True(t, strings.HasPrefix(resp.RequestID, "req_"), resp.RequestID)
	assert.Equal(t, "auto", resp.ResolvedSearchType)
	assert.GreaterOrEqual(t, resp.SearchTime, 0.0)
	require.Len(t, resp.Results, 2)
	a := resp.Results[0]
	assert.Equal(t, "https://example.com/a", a.URL)
	assert.Equal(t, websearch.DocumentID("https://example.com/a"), a.ID)
	assert.Equal(t, "A", a.Title)
	assert.Equal(t, &published, a.PublishedDate)
	assert.Equal(t, "https://example.com/a.png", a.Image)
	assert.Nil(t, a.Text)
	assert.Equal(t, "https://b.com/", resp.Results[1].URL)
	assert.Empty(t, resp.Warnings)
}

func TestSearchTypes(t *testing.T) {
	tests := []struct {
		typ          string
		additional   []string
		expansions   int // 0: ExpandQuery not called
		wantQueries  []string
		wantResolved string
	}{
		{"", nil, 0, []string{"q"}, "auto"},
		{"instant", nil, 0, []string{"q"}, "auto"},
		{"lite", nil, 0, []string{"q"}, "lite"},
		{"deep", []string{"extra"}, 4, []string{"q", "extra", "x1", "x2"}, "deep"},
		{"deep-reasoning", nil, 4, []string{"q", "x1", "x2"}, "deep"},
		{"deep-lite", nil, 2, []string{"q", "x1", "x2"}, "deep-lite"},
	}
	for _, tc := range tests {
		t.Run(tc.typ, func(t *testing.T) {
			e := newEnv(t)
			if tc.expansions > 0 {
				e.synth.EXPECT().ExpandQuery(gomock.Any(), "q", "obj", tc.expansions).Return([]string{"x1", "x2"}, nil)
			}
			got := e.discover()
			resp, err := e.provider(t).Search(context.Background(), &websearch.SearchRequest{
				Query: "q", Type: tc.typ, AdditionalQueries: tc.additional, Objective: "obj",
			})
			require.NoError(t, err)
			assert.Equal(t, tc.wantQueries, got.Queries)
			assert.Equal(t, tc.wantResolved, resp.ResolvedSearchType)
			assert.NotNil(t, resp.Results, "results serialise as [] when empty")
		})
	}
}

func TestSearchRejects(t *testing.T) {
	schema := json.RawMessage(`{"type":"text"}`)
	tests := []struct {
		name     string
		noRanker bool
		noSynth  bool
		req      websearch.SearchRequest
		tag      string
	}{
		{"invalid request", false, false, websearch.SearchRequest{Query: "q", NumResults: 101}, websearch.TagInvalidRequest},
		{"unknown type", false, false, websearch.SearchRequest{Query: "q", Type: "neural"}, websearch.TagFeatureDisabled},
		{"compliance", false, false, websearch.SearchRequest{Query: "q", Compliance: ptr("hipaa")}, websearch.TagFeatureDisabled},
		{"deep without LLM", false, true, websearch.SearchRequest{Query: "q", Type: "deep"}, websearch.TagFeatureDisabled},
		{"outputSchema without LLM", false, true, websearch.SearchRequest{Query: "q", OutputSchema: schema}, websearch.TagFeatureDisabled},
		{"summary without LLM", false, true, websearch.SearchRequest{Query: "q", Contents: &websearch.ContentsOptions{Summary: &websearch.SummaryOptions{}}}, websearch.TagFeatureDisabled},
		{"highlights without ranker", true, false, websearch.SearchRequest{Query: "q", Contents: &websearch.ContentsOptions{Highlights: &websearch.HighlightsOption{Enabled: true}}}, websearch.TagFeatureDisabled},
		{"dynamic highlights", false, false, websearch.SearchRequest{Query: "q", Contents: &websearch.ContentsOptions{Highlights: &websearch.HighlightsOption{Enabled: true, HighlightsOptions: websearch.HighlightsOptions{Dynamic: true}}}}, websearch.TagFeatureDisabled},
		{"highlights verbosity", false, false, websearch.SearchRequest{Query: "q", Contents: &websearch.ContentsOptions{Highlights: &websearch.HighlightsOption{Enabled: true, HighlightsOptions: websearch.HighlightsOptions{Verbosity: "full"}}}}, websearch.TagFeatureDisabled},
		{"snapshotAsOf", false, false, websearch.SearchRequest{Query: "q", Contents: &websearch.ContentsOptions{SnapshotAsOf: "2025-01-01"}}, websearch.TagFeatureDisabled},
		{"crawledBeforeDate", false, false, websearch.SearchRequest{Query: "q", Contents: &websearch.ContentsOptions{CrawledBefore: "2025-01-01"}}, websearch.TagFeatureDisabled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			if tc.noRanker {
				e.opts.Ranker = nil
			}
			if tc.noSynth {
				e.opts.Synthesizer = nil
			}
			// No component expectations: rejecting must happen before any work.
			_, err := e.provider(t).Search(context.Background(), &tc.req)
			requireTag(t, err, tc.tag)
		})
	}
}

func TestSearchDiscoverError(t *testing.T) {
	e := newEnv(t)
	e.disc.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(nil, errors.New("searxng down"))
	_, err := e.provider(t).Search(context.Background(), &websearch.SearchRequest{Query: "q"})
	require.ErrorContains(t, err, "searxng down")
}

func TestSearchContents(t *testing.T) {
	e := newEnv(t)
	published := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	e.pipeline(map[string]error{
		"https://b.com/": &FetchError{Tag: websearch.TagCrawlHTTPError, HTTPStatusCode: 500, Err: errors.New("boom")},
	})
	e.pages["https://a.com/"] = &Page{Title: "Page A", Author: "Ann", PublishedDate: &published, Image: "img", Favicon: "fav"}
	e.bodies["https://c.com/"] = strings.Repeat("é", 12_000)
	e.discover(cands("https://a.com/", "https://b.com/", "https://c.com/")...)

	resp, err := e.provider(t).Search(context.Background(), &websearch.SearchRequest{
		Query:    "q",
		Contents: &websearch.ContentsOptions{Text: &websearch.TextOption{Enabled: true}},
	})
	require.NoError(t, err)
	require.Len(t, resp.Results, 3, "a result whose page failed is still returned")

	a := resp.Results[0]
	assert.Equal(t, "Candidate https://a.com/", a.Title, "the discovered title is kept")
	assert.Equal(t, "Ann", *a.Author)
	assert.Equal(t, &published, a.PublishedDate, "contents fill a missing date")
	assert.Equal(t, "img", a.Image)
	assert.Equal(t, "fav", a.Favicon)
	assert.Equal(t, bodyOf("https://a.com/"), *a.Text)

	assert.Nil(t, resp.Results[1].Text)
	assert.Nil(t, resp.Results[1].Author)
	assert.Equal(t, 10_000, len([]rune(*resp.Results[2].Text)), "search text defaults to 10,000 characters")

	require.Len(t, resp.Warnings, 1)
	assert.Equal(t, websearch.WarningApproximatedField, resp.Warnings[0].Code)
	assert.Contains(t, resp.Warnings[0].Message, "1 of 3 results")
	assert.ElementsMatch(t, []string{"https://a.com/", "https://c.com/"}, e.puts, "fetched pages are cached")
}

func TestSearchFetchInfrastructureErrorFails(t *testing.T) {
	e := newEnv(t)
	e.memCache()
	e.fetch.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("browser pool closed"))
	e.discover(cands("https://a.com/")...)
	_, err := e.provider(t).Search(context.Background(), &websearch.SearchRequest{
		Query: "q", Contents: &websearch.ContentsOptions{Text: &websearch.TextOption{Enabled: true}},
	})
	require.ErrorContains(t, err, "browser pool closed")
}

func TestSearchTextFilters(t *testing.T) {
	e := newEnv(t)
	e.pipeline(map[string]error{"https://d.com/": &FetchError{Tag: websearch.TagCrawlTimeout, Err: errors.New("slow")}})
	e.bodies["https://a.com/"] = "Go generics are GREAT"
	e.bodies["https://b.com/"] = "go generics are slow"
	e.bodies["https://c.com/"] = "rust traits"
	e.discover(cands("https://a.com/", "https://b.com/", "https://c.com/", "https://d.com/")...)

	resp, err := e.provider(t).Search(context.Background(), &websearch.SearchRequest{
		Query: "q", IncludeText: []string{"generics"}, ExcludeText: []string{"slow"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://a.com/"}, resultURLs(resp.Results), "unfetched results cannot pass a text filter")
	assert.Nil(t, resp.Results[0].Text, "filters fetch text without returning it")

	paths := map[string]string{}
	for _, w := range resp.Warnings {
		paths[w.Path] = w.Code
	}
	assert.Equal(t, websearch.WarningApproximatedField, paths["includeText"])
	assert.Equal(t, websearch.WarningApproximatedField, paths["excludeText"])
	assert.Equal(t, websearch.WarningApproximatedField, paths["contents"])
}

func TestSearchWarnings(t *testing.T) {
	e := newEnv(t)
	e.discover()
	day := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	resp, err := e.provider(t).Search(context.Background(), &websearch.SearchRequest{
		Query: "q", StartCrawlDate: &day, EndCrawlDate: &day,
		UserLocation: &websearch.UserLocation{Country: "US", Latitude: ptr(1.0), Longitude: ptr(2.0)},
		Objective:    "o", SystemPrompt: "s",
		Contents: &websearch.ContentsOptions{
			Extras:     &websearch.ExtrasOptions{RichLinks: 1, CodeBlocks: 1},
			Highlights: &websearch.HighlightsOption{Enabled: true, HighlightsOptions: websearch.HighlightsOptions{HighlightsPerURL: 2}},
		},
	})
	require.NoError(t, err)
	var paths []string
	for _, w := range resp.Warnings {
		assert.Equal(t, websearch.WarningIgnoredField, w.Code, w.Path)
		paths = append(paths, w.Path)
	}
	assert.Equal(t, []string{"startCrawlDate", "endCrawlDate", "userLocation", "objective", "systemPrompt",
		"contents.highlights.highlightsPerUrl", "contents.extras.richLinks", "contents.extras.codeBlocks"}, paths)
}

func TestSearchContext(t *testing.T) {
	for name, req := range map[string]websearch.SearchRequest{
		"top level":       {Query: "q", Context: &websearch.ContextOption{Enabled: true}},
		"inside contents": {Query: "q", Contents: &websearch.ContentsOptions{Context: &websearch.ContextOption{Enabled: true}}},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.pipeline(nil)
			e.discover(cands("https://a.com/", "https://b.com/")...)
			resp, err := e.provider(t).Search(context.Background(), &req)
			require.NoError(t, err)
			assert.Equal(t, "Title: Candidate https://a.com/\nURL: https://a.com/\ntext of https://a.com/\n\n"+
				"Title: Candidate https://b.com/\nURL: https://b.com/\ntext of https://b.com/", resp.Context)
		})
	}
	t.Run("maxCharacters", func(t *testing.T) {
		e := newEnv(t)
		e.pipeline(nil)
		e.discover(cands("https://a.com/")...)
		resp, err := e.provider(t).Search(context.Background(), &websearch.SearchRequest{
			Query: "q", Context: &websearch.ContextOption{Enabled: true, ContextOptions: websearch.ContextOptions{MaxCharacters: 5}},
		})
		require.NoError(t, err)
		assert.Equal(t, "Title", resp.Context)
	})
}

func TestSearchHighlights(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.discover(cands("https://a.com/")...)
	e.rank.EXPECT().Rank(gomock.Any(), "q", []string{bodyOf("https://a.com/")}).Return([]float64{0.7}, nil)
	resp, err := e.provider(t).Search(context.Background(), &websearch.SearchRequest{
		Query: "q", Contents: &websearch.ContentsOptions{Highlights: &websearch.HighlightsOption{Enabled: true}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{bodyOf("https://a.com/")}, resp.Results[0].Highlights)
	assert.Nil(t, resp.Results[0].HighlightScores, "Exa returns highlightScores on findSimilar only")
	assert.Nil(t, resp.Results[0].Text)
}

func TestHighlightsMaxCharacters(t *testing.T) {
	long := strings.Repeat("word ", 1000) // one 5000-char passage
	tests := []struct {
		name string
		opts websearch.HighlightsOptions
		want int
	}{
		{"maxCharacters", websearch.HighlightsOptions{Query: "own", MaxCharacters: 100}, 100},
		{"numSentences", websearch.HighlightsOptions{Query: "own", NumSentences: 2}, 2 * charsPerSentence},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.pipeline(nil)
			e.bodies["https://a.com/"] = long
			e.discover(cands("https://a.com/")...)
			e.rank.EXPECT().Rank(gomock.Any(), "own", gomock.Any()).DoAndReturn(
				func(_ context.Context, _ string, passages []string) ([]float64, error) {
					return make([]float64, len(passages)), nil
				})
			resp, err := e.provider(t).Search(context.Background(), &websearch.SearchRequest{
				Query: "q", Contents: &websearch.ContentsOptions{Highlights: &websearch.HighlightsOption{Enabled: true, HighlightsOptions: tc.opts}},
			})
			require.NoError(t, err)
			require.NotEmpty(t, resp.Results[0].Highlights)
			assert.LessOrEqual(t, len(strings.Join(resp.Results[0].Highlights, "")), tc.want)
			assert.Greater(t, len(strings.Join(resp.Results[0].Highlights, "")), tc.want-maxPassageChars, "highlights fill the budget to within one passage")
		})
	}
}

func TestSearchOutputSchema(t *testing.T) {
	e := newEnv(t)
	e.pipeline(map[string]error{"https://b.com/": &FetchError{Tag: websearch.TagCrawlNotFound, Err: errors.New("404")}})
	e.discover(cands("https://a.com/", "https://b.com/")...)
	schema := json.RawMessage(`{"type":"text"}`)
	out := &websearch.SearchOutput{Content: "answer [1]", Grounding: []websearch.Grounding{{Field: "content", Confidence: "low"}}}
	e.synth.EXPECT().Synthesize(gomock.Any(), &SearchSynthesis{
		Query: "q", Objective: "o", SystemPrompt: "s", OutputSchema: schema,
		Sources: []Source{{URL: "https://a.com/", Title: "Candidate https://a.com/", Text: bodyOf("https://a.com/")}},
	}, gomock.Nil()).Return(out, nil)

	resp, err := e.provider(t).Search(context.Background(), &websearch.SearchRequest{
		Query: "q", Objective: "o", SystemPrompt: "s", OutputSchema: schema,
	})
	require.NoError(t, err)
	assert.Equal(t, out, resp.Output)
	assert.Equal(t, []string{"<nil>", "<nil>"}, resultTexts(resp.Results), "sources are fetched without returning text")
}

func TestSearchOutputSchemaError(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.discover(cands("https://a.com/")...)
	e.synth.EXPECT().Synthesize(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("llm down"))
	_, err := e.provider(t).Search(context.Background(), &websearch.SearchRequest{Query: "q", OutputSchema: json.RawMessage(`{"type":"text"}`)})
	require.ErrorContains(t, err, "llm down")
}

func TestStreamSearch(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.discover(cands("https://a.com/")...)
	out := &websearch.SearchOutput{Content: "ab", Grounding: []websearch.Grounding{{Field: "content", Confidence: "high"}}}
	e.synth.EXPECT().Synthesize(gomock.Any(), gomock.Any(), gomock.Not(gomock.Nil())).DoAndReturn(
		func(_ context.Context, _ *SearchSynthesis, onDelta func(string) error) (*websearch.SearchOutput, error) {
			require.NoError(t, onDelta("a"))
			require.NoError(t, onDelta("b"))
			return out, nil
		})

	var events []websearch.SearchEvent
	err := e.provider(t).StreamSearch(context.Background(), &websearch.SearchRequest{
		Query: "q", Stream: true, OutputSchema: json.RawMessage(`{"type":"text"}`), StartCrawlDate: ptr(time.Now()),
	}, func(ev websearch.SearchEvent) error {
		events = append(events, ev)
		return nil
	})
	require.NoError(t, err)

	var types []string
	for _, ev := range events {
		types = append(types, ev.Type)
	}
	assert.Equal(t, []string{"results", "text-delta", "text-delta", "grounding", "done"}, types)
	assert.Equal(t, "https://a.com/", events[0].Results[0].URL)
	assert.Equal(t, "a", events[1].Delta)
	assert.Equal(t, out.Grounding, events[3].Grounding)
	done := events[4]
	assert.Equal(t, out, done.Output)
	require.Len(t, done.Warnings, 1)
	assert.Equal(t, "startCrawlDate", done.Warnings[0].Path)
}

func TestStreamSearchEmitErrorStops(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.discover(cands("https://a.com/")...)
	err := e.provider(t).StreamSearch(context.Background(), &websearch.SearchRequest{
		Query: "q", OutputSchema: json.RawMessage(`{"type":"text"}`),
	}, func(websearch.SearchEvent) error { return errors.New("client gone") })
	require.ErrorContains(t, err, "client gone")
}

func contentsReq(opts websearch.ContentsOptions, urls ...string) *websearch.ContentsRequest {
	return &websearch.ContentsRequest{URLs: urls, ContentsOptions: opts}
}

func TestContentsStatuses(t *testing.T) {
	e := newEnv(t)
	e.pipeline(map[string]error{
		"https://b.com/": &FetchError{Tag: websearch.TagCrawlHTTPError, HTTPStatusCode: 404, Err: errors.New("not found")},
		"https://c.com/": &FetchError{Tag: websearch.TagBlockedDestination, Err: errors.New("private address")},
	})
	e.cachePage("https://d.com/", time.Hour)

	resp, err := e.provider(t).Contents(context.Background(), contentsReq(
		websearch.ContentsOptions{Text: &websearch.TextOption{Enabled: true}},
		"https://A.com", "https://b.com/", "https://c.com/", "https://d.com/"))
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(resp.RequestID, "req_"))
	assert.Equal(t, []websearch.ContentStatus{
		{ID: "https://A.com", Status: "success", Source: "crawled"},
		{ID: "https://b.com/", Status: "error", Error: &websearch.ContentError{Tag: websearch.TagCrawlHTTPError, HTTPStatusCode: 404}},
		{ID: "https://c.com/", Status: "error", Error: &websearch.ContentError{Tag: websearch.TagBlockedDestination}},
		{ID: "https://d.com/", Status: "success", Source: "cached"},
	}, resp.Statuses)
	assert.Equal(t, []string{"https://a.com/", "https://d.com/"}, resultURLs(resp.Results), "results are the successes, in order")
	assert.Equal(t, websearch.DocumentID("https://a.com/"), resp.Results[0].ID)
	assert.Equal(t, "Page https://a.com/", resp.Results[0].Title)
	assert.Equal(t, "Ann", *resp.Results[0].Author)
	assert.Equal(t, []string{bodyOf("https://a.com/"), "cached " + bodyOf("https://d.com/")}, resultTexts(resp.Results))
	assert.Equal(t, []string{"https://a.com/"}, e.puts, "only fetched pages are written to the cache")
}

func TestContentsByID(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.cachePage("https://a.com/", time.Hour)
	known := websearch.DocumentID("https://a.com/")

	resp, err := e.provider(t).Contents(context.Background(), &websearch.ContentsRequest{
		IDs: []string{known, "doc_unknown"}, ContentsOptions: websearch.ContentsOptions{Text: &websearch.TextOption{Enabled: true}},
	})
	require.NoError(t, err)
	assert.Equal(t, []websearch.ContentStatus{
		{ID: known, Status: "success", Source: "cached"},
		{ID: "doc_unknown", Status: "error", Error: &websearch.ContentError{Tag: websearch.TagSourceNotAvailable}},
	}, resp.Statuses)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, known, resp.Results[0].ID)
	assert.Equal(t, "https://a.com/", resp.Results[0].URL)
	assert.Empty(t, e.fetched)
}

func TestContentsByIDRefetchesStalePage(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.cachePage("https://a.com/", 5*time.Hour)
	resp, err := e.provider(t).Contents(context.Background(), &websearch.ContentsRequest{
		IDs: []string{websearch.DocumentID("https://a.com/")}, ContentsOptions: websearch.ContentsOptions{MaxAgeHours: ptr(1)},
	})
	require.NoError(t, err)
	assert.Equal(t, "crawled", resp.Statuses[0].Source)
	assert.Equal(t, []string{"https://a.com/"}, e.fetched)
}

func TestContentsFreshness(t *testing.T) {
	const u = "https://a.com/"
	tests := []struct {
		name       string
		opts       websearch.ContentsOptions
		cachedAge  time.Duration // 0: not cached
		wantSource string        // "" = error
		wantTag    string
		wantGet    bool
	}{
		{"default uses cache", websearch.ContentsOptions{}, 1000 * time.Hour, "cached", "", true},
		{"default fetches on miss", websearch.ContentsOptions{}, 0, "crawled", "", true},
		{"-1 uses cache", websearch.ContentsOptions{MaxAgeHours: ptr(-1)}, 1000 * time.Hour, "cached", "", true},
		{"-1 miss is an error", websearch.ContentsOptions{MaxAgeHours: ptr(-1)}, 0, "", websearch.TagSourceNotAvailable, true},
		{"0 always fetches", websearch.ContentsOptions{MaxAgeHours: ptr(0)}, time.Minute, "crawled", "", false},
		{"N fresh uses cache", websearch.ContentsOptions{MaxAgeHours: ptr(24)}, time.Hour, "cached", "", true},
		{"N stale fetches", websearch.ContentsOptions{MaxAgeHours: ptr(1)}, 2 * time.Hour, "crawled", "", true},
		{"livecrawl never", websearch.ContentsOptions{Livecrawl: "never"}, 0, "", websearch.TagSourceNotAvailable, true},
		{"livecrawl always", websearch.ContentsOptions{Livecrawl: "always"}, time.Minute, "crawled", "", false},
		{"livecrawl fallback", websearch.ContentsOptions{Livecrawl: "fallback"}, 1000 * time.Hour, "cached", "", true},
		{"livecrawl preferred", websearch.ContentsOptions{Livecrawl: "preferred"}, 1000 * time.Hour, "cached", "", true},
		{"livecrawl auto", websearch.ContentsOptions{Livecrawl: "auto"}, 0, "crawled", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.web(nil)
			e.extract()
			if tc.cachedAge > 0 {
				e.cachePage(u, tc.cachedAge)
			}
			getCalls := 0
			e.cache.EXPECT().Get(gomock.Any(), u).DoAndReturn(func(context.Context, string) (*RawPage, error) {
				getCalls++
				return e.cached[u], nil
			}).AnyTimes()
			e.cache.EXPECT().Put(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

			resp, err := e.provider(t).Contents(context.Background(), contentsReq(tc.opts, u))
			require.NoError(t, err)
			st := resp.Statuses[0]
			if tc.wantSource == "" {
				assert.Equal(t, "error", st.Status)
				assert.Equal(t, tc.wantTag, st.Error.Tag)
				assert.Empty(t, resp.Results)
			} else {
				assert.Equal(t, "success", st.Status)
				assert.Equal(t, tc.wantSource, st.Source)
			}
			assert.Equal(t, tc.wantGet, getCalls > 0, "cache consulted")
		})
	}
}

func TestContentsFetchTimeout(t *testing.T) {
	for name, tc := range map[string]struct {
		ms   int
		want time.Duration
	}{
		"default": {0, 10 * time.Second},
		"custom":  {2500, 2500 * time.Millisecond},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.memCache()
			e.extract()
			e.fetch.EXPECT().Fetch(gomock.Any(), "https://a.com/", tc.want).Return(&RawPage{URL: "https://a.com/", Body: []byte("x")}, nil)
			_, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{LivecrawlTimeout: tc.ms}, "https://a.com/"))
			require.NoError(t, err)
		})
	}
}

func TestContentsExtractErrors(t *testing.T) {
	t.Run("unsupported document is a status", func(t *testing.T) {
		e := newEnv(t)
		e.web(nil)
		e.memCache()
		e.ext.EXPECT().Extract(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.Join(ErrUnsupportedDocument, errors.New("image/png")))
		resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{}, "https://a.com/x.png"))
		require.NoError(t, err)
		assert.Equal(t, &websearch.ContentError{Tag: websearch.TagUnsupportedDocument}, resp.Statuses[0].Error)
	})
	t.Run("other extract errors fail the request", func(t *testing.T) {
		e := newEnv(t)
		e.web(nil)
		e.memCache()
		e.ext.EXPECT().Extract(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("extractor service down"))
		_, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{}, "https://a.com/"))
		require.ErrorContains(t, err, "extractor service down")
	})
	t.Run("cache errors fail the request", func(t *testing.T) {
		e := newEnv(t)
		e.cache.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, errors.New("db down"))
		_, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{}, "https://a.com/"))
		require.ErrorContains(t, err, "db down")
	})
}

func TestContentsRejects(t *testing.T) {
	tests := []struct {
		name     string
		noRanker bool
		noSynth  bool
		req      *websearch.ContentsRequest
		tag      string
	}{
		{"invalid", false, false, &websearch.ContentsRequest{}, websearch.TagInvalidRequest},
		{"compliance", false, false, &websearch.ContentsRequest{URLs: []string{"https://a.com"}, Compliance: ptr("hipaa")}, websearch.TagFeatureDisabled},
		{"snapshotAsOf", false, false, contentsReq(websearch.ContentsOptions{SnapshotAsOf: "2025-01-01"}, "https://a.com"), websearch.TagFeatureDisabled},
		{"summary without LLM", false, true, contentsReq(websearch.ContentsOptions{Summary: &websearch.SummaryOptions{}}, "https://a.com"), websearch.TagFeatureDisabled},
		{"highlights without ranker", true, false, contentsReq(websearch.ContentsOptions{Highlights: &websearch.HighlightsOption{Enabled: true}}, "https://a.com"), websearch.TagFeatureDisabled},
		{"bad url", false, false, contentsReq(websearch.ContentsOptions{}, "http://[::1"), websearch.TagInvalidRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			if tc.noRanker {
				e.opts.Ranker = nil
			}
			if tc.noSynth {
				e.opts.Synthesizer = nil
			}
			_, err := e.provider(t).Contents(context.Background(), tc.req)
			requireTag(t, err, tc.tag)
		})
	}
}

func TestContentsText(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	big := strings.Repeat("x", 20_000)
	e.bodies["https://a.com/"] = big
	e.bodies["https://b.com/"] = big
	p := e.provider(t)

	resp, err := p.Contents(context.Background(), contentsReq(websearch.ContentsOptions{Text: &websearch.TextOption{Enabled: true}}, "https://a.com/"))
	require.NoError(t, err)
	assert.Len(t, *resp.Results[0].Text, 20_000, "/contents defaults to 5,000,000 characters")

	resp, err = p.Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Text: &websearch.TextOption{Enabled: true, TextOptions: websearch.TextOptions{MaxCharacters: 7}}}, "https://b.com/"))
	require.NoError(t, err)
	assert.Equal(t, "xxxxxxx", *resp.Results[0].Text)
}

func TestContentsTextOptionsReachExtractor(t *testing.T) {
	e := newEnv(t)
	e.web(nil)
	e.memCache()
	opts := websearch.TextOptions{IncludeHTMLTags: true, Verbosity: "full"}
	e.ext.EXPECT().Extract(gomock.Any(), gomock.Any(), opts).Return(&Page{Text: "<p>x</p>"}, nil)
	resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Text: &websearch.TextOption{Enabled: true, TextOptions: opts}}, "https://a.com/"))
	require.NoError(t, err)
	assert.Equal(t, "<p>x</p>", *resp.Results[0].Text)
}

func TestContentsHighlightsQuery(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.rank.EXPECT().Rank(gomock.Any(), "Page https://a.com/", gomock.Any()).Return([]float64{0.3}, nil)
	e.rank.EXPECT().Rank(gomock.Any(), "mine", gomock.Any()).Return([]float64{0.4}, nil)
	p := e.provider(t)

	resp, err := p.Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Highlights: &websearch.HighlightsOption{Enabled: true}}, "https://a.com/"))
	require.NoError(t, err, "without a query, highlights rank against the page title")
	assert.NotEmpty(t, resp.Results[0].Highlights)

	_, err = p.Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Highlights: &websearch.HighlightsOption{Enabled: true, HighlightsOptions: websearch.HighlightsOptions{Query: "mine"}}}, "https://a.com/"))
	require.NoError(t, err)
}

func TestContentsHighlightsRankerError(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.rank.EXPECT().Rank(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("embeddings down"))
	_, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Highlights: &websearch.HighlightsOption{Enabled: true}}, "https://a.com/"))
	require.ErrorContains(t, err, "embeddings down")
}

func TestContentsSummary(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	schema := json.RawMessage(`{"type":"object"}`)
	e.synth.EXPECT().Summarize(gomock.Any(), bodyOf("https://a.com/"), "focus", schema).Return(`{"a":1}`, nil)
	resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Summary: &websearch.SummaryOptions{Query: "focus", Schema: schema}}, "https://a.com/"))
	require.NoError(t, err)
	assert.Equal(t, `{"a":1}`, *resp.Results[0].Summary)
}

func TestContentsExtras(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.pages["https://a.com/"] = &Page{
		Links:      []Link{{URL: "https://a.com/1"}, {URL: "https://a.com/2"}, {URL: "https://x.com/"}},
		ImageLinks: []websearch.ImageLink{{URL: "https://a.com/i1.png", Alt: "one"}, {URL: "https://a.com/i2.png"}},
	}
	resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Extras: &websearch.ExtrasOptions{Links: 2, ImageLinks: 5, RichImageLinks: 1}}, "https://a.com/"))
	require.NoError(t, err)
	assert.Equal(t, &websearch.Extras{
		Links:          []string{"https://a.com/1", "https://a.com/2"},
		ImageLinks:     []string{"https://a.com/i1.png", "https://a.com/i2.png"},
		RichImageLinks: []websearch.ImageLink{{URL: "https://a.com/i1.png", Alt: "one"}},
	}, resp.Results[0].Extras)
}

func TestContentsSubpages(t *testing.T) {
	e := newEnv(t)
	e.pipeline(map[string]error{"https://a.com/pricing": &FetchError{Tag: websearch.TagCrawlNotFound, Err: errors.New("404")}})
	e.pages["https://a.com/"] = &Page{Title: "Home", Links: []Link{
		{URL: "https://a.com/"},                              // itself
		{URL: "https://other.com/docs"},                      // other host
		{URL: "https://a.com/blog"},                          // no match
		{URL: "https://a.com/about"},                         // no match
		{URL: "https://a.com/docs#x"},                        // matches "docs"
		{URL: "https://a.com/docs"},                          // duplicate after normalisation
		{URL: "https://a.com/pricing"},                       // matches, fails to fetch
		{URL: "https://a.com/p", Anchor: "Docs and pricing"}, // matches both targets via anchor
	}}
	resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Text: &websearch.TextOption{Enabled: true}, Subpages: 4, SubpageTarget: websearch.StringList{"docs", "pricing"},
	}, "https://a.com/"))
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	sub := resp.Results[0].Subpages
	assert.Equal(t, []string{"https://a.com/p", "https://a.com/docs", "https://a.com/blog"}, resultURLs(sub),
		"best target match first, then page order; a failed subpage is left out")
	assert.Equal(t, bodyOf("https://a.com/docs"), *sub[1].Text, "subpages get the same contents")
	assert.Equal(t, websearch.DocumentID("https://a.com/docs"), sub[1].ID)
	assert.Nil(t, sub[0].Subpages)
	assert.Len(t, resp.Statuses, 1, "statuses cover the requested pages only")
}

func TestContentsContext(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Context: &websearch.ContextOption{Enabled: true}}, "https://a.com/"))
	require.NoError(t, err)
	assert.Equal(t, "Title: Page https://a.com/\nURL: https://a.com/\ntext of https://a.com/", resp.Context)
}

func TestFetchConcurrencyBound(t *testing.T) {
	e := newEnv(t)
	e.opts.FetchConcurrency = 3
	e.memCache()
	e.extract()
	var inFlight, peak atomic.Int32
	e.fetch.EXPECT().Fetch(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, u string, _ time.Duration) (*RawPage, error) {
			n := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			return &RawPage{URL: u, Body: []byte("x"), FetchedAt: time.Now()}, nil
		}).Times(12)
	p := e.provider(t)

	var targets []string
	for i := range 12 {
		targets = append(targets, "https://a.com/"+string(rune('a'+i)))
	}
	// Two requests at once share the provider's fetch slots.
	var wg sync.WaitGroup
	for _, half := range [][]string{targets[:6], targets[6:]} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := p.Contents(context.Background(), contentsReq(websearch.ContentsOptions{}, half...))
			assert.NoError(t, err)
			assert.Len(t, resp.Results, 6)
		}()
	}
	wg.Wait()
	assert.LessOrEqual(t, peak.Load(), int32(3))
	assert.Equal(t, int32(3), peak.Load(), "fetches do run in parallel")
}

func TestFindSimilar(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.pages["https://src.com/post"] = &Page{Title: "Source", Text: "about rust"}
	e.synth.EXPECT().Describe(gomock.Any(), "Source", "about rust").Return("rust articles", nil)
	got := e.discover(cands("https://src.com/post", "https://www.src.com/other", "https://b.com/", "https://c.com/")...)
	e.rank.EXPECT().Rank(gomock.Any(), "rust articles",
		[]string{"Candidate https://b.com/\nsnippet https://b.com/", "Candidate https://c.com/\nsnippet https://c.com/"}).
		Return([]float64{0.2, 0.9}, nil)
	e.rank.EXPECT().Rank(gomock.Any(), "rust articles", gomock.Any()).Return([]float64{0.5}, nil).Times(2)

	resp, err := e.provider(t).FindSimilar(context.Background(), &websearch.FindSimilarRequest{
		URL: "https://src.com/post", NumResults: 2, ExcludeSourceDomain: true, Category: "news",
		Contents: &websearch.ContentsOptions{Highlights: &websearch.HighlightsOption{Enabled: true}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"rust articles"}, got.Queries)
	assert.Equal(t, 3, got.NumResults, "one extra, for the source page")
	assert.Equal(t, "news", got.Category)
	assert.Equal(t, []string{"https://c.com/", "https://b.com/"}, resultURLs(resp.Results), "sorted by score")
	assert.Equal(t, 0.9, *resp.Results[0].Score)
	assert.Equal(t, []float64{0.5}, resp.Results[0].HighlightScores)
	assert.True(t, strings.HasPrefix(resp.RequestID, "req_"))
}

func TestFindSimilarKeepsSourceDomainByDefault(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.synth.EXPECT().Describe(gomock.Any(), gomock.Any(), gomock.Any()).Return("d", nil)
	e.discover(cands("https://src.com/", "https://src.com/other")...)
	e.rank.EXPECT().Rank(gomock.Any(), "d", gomock.Len(1)).Return([]float64{0.1}, nil)
	resp, err := e.provider(t).FindSimilar(context.Background(), &websearch.FindSimilarRequest{URL: "https://src.com"})
	require.NoError(t, err)
	assert.Equal(t, []string{"https://src.com/other"}, resultURLs(resp.Results), "only the source page itself is dropped")
}

func TestFindSimilarFailures(t *testing.T) {
	t.Run("needs LLM and ranker", func(t *testing.T) {
		for _, drop := range []func(*Options){
			func(o *Options) { o.Synthesizer = nil },
			func(o *Options) { o.Ranker = nil },
		} {
			e := newEnv(t)
			drop(&e.opts)
			_, err := e.provider(t).FindSimilar(context.Background(), &websearch.FindSimilarRequest{URL: "https://a.com"})
			requireTag(t, err, websearch.TagFeatureDisabled)
		}
	})
	t.Run("source cannot be fetched", func(t *testing.T) {
		e := newEnv(t)
		e.pipeline(map[string]error{"https://a.com/": &FetchError{Tag: websearch.TagCrawlTimeout, Err: errors.New("slow")}})
		_, err := e.provider(t).FindSimilar(context.Background(), &websearch.FindSimilarRequest{URL: "https://a.com"})
		requireTag(t, err, websearch.TagInvalidRequest)
		assert.Contains(t, err.Error(), websearch.TagCrawlTimeout)
	})
	t.Run("ranker score count mismatch", func(t *testing.T) {
		e := newEnv(t)
		e.pipeline(nil)
		e.synth.EXPECT().Describe(gomock.Any(), gomock.Any(), gomock.Any()).Return("d", nil)
		e.discover(cands("https://b.com/", "https://c.com/")...)
		e.rank.EXPECT().Rank(gomock.Any(), gomock.Any(), gomock.Any()).Return([]float64{0.1}, nil)
		_, err := e.provider(t).FindSimilar(context.Background(), &websearch.FindSimilarRequest{URL: "https://a.com"})
		require.ErrorContains(t, err, "1 scores for 2 pages")
	})
}

func TestAnswerModels(t *testing.T) {
	tests := []struct {
		model string
		tier  Tier
		deep  bool
	}{
		{"", TierDefault, false},
		{"exa", TierDefault, false},
		{"exa-fast", TierFast, false},
		{"exa-pro", TierStrong, false},
		{"exa-research", TierStrong, true},
	}
	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			e := newEnv(t)
			e.pipeline(nil)
			if tc.deep {
				e.synth.EXPECT().ExpandQuery(gomock.Any(), "q", "", deepExpansions).Return(nil, nil)
			}
			got := e.discover(cands("https://a.com/")...)
			e.synth.EXPECT().Answer(gomock.Any(), gomock.Any(), gomock.Nil()).DoAndReturn(
				func(_ context.Context, a *AnswerSynthesis, _ func(string) error) (any, error) {
					assert.Equal(t, tc.tier, a.Tier)
					return "42", nil
				})
			_, err := e.provider(t).Answer(context.Background(), &websearch.AnswerRequest{Query: "q", Model: tc.model})
			require.NoError(t, err)
			assert.Equal(t, answerNumResults, got.NumResults)
		})
	}
}

func TestAnswer(t *testing.T) {
	for _, withText := range []bool{false, true} {
		t.Run(map[bool]string{false: "without text", true: "with text"}[withText], func(t *testing.T) {
			e := newEnv(t)
			e.pipeline(map[string]error{"https://b.com/": &FetchError{Tag: websearch.TagCrawlNotFound, Err: errors.New("404")}})
			got := e.discover(cands("https://a.com/", "https://b.com/")...)
			schema := json.RawMessage(`{"type":"object"}`)
			e.synth.EXPECT().Answer(gomock.Any(), &AnswerSynthesis{
				Tier: TierDefault, Query: "q", SystemPrompt: "sys", OutputSchema: schema,
				Sources: []Source{{URL: "https://a.com/", Title: "Candidate https://a.com/", Text: bodyOf("https://a.com/")}},
			}, gomock.Nil()).Return(map[string]any{"x": 1.0}, nil)

			resp, err := e.provider(t).Answer(context.Background(), &websearch.AnswerRequest{
				Query: "q", SystemPrompt: "sys", OutputSchema: schema, Text: withText,
				UserLocation: &websearch.UserLocation{Country: "FR"},
			})
			require.NoError(t, err)
			assert.Equal(t, "FR", got.Country)
			assert.Equal(t, map[string]any{"x": 1.0}, resp.Answer)
			assert.True(t, strings.HasPrefix(resp.RequestID, "req_"))
			require.Len(t, resp.Citations, 1, "citations are the sources the answer was given")
			assert.Equal(t, "https://a.com/", resp.Citations[0].URL)
			if withText {
				assert.Equal(t, bodyOf("https://a.com/"), *resp.Citations[0].Text)
			} else {
				assert.Nil(t, resp.Citations[0].Text)
			}
		})
	}
}

func TestAnswerFailures(t *testing.T) {
	t.Run("needs LLM", func(t *testing.T) {
		e := newEnv(t)
		e.opts.Synthesizer = nil
		_, err := e.provider(t).Answer(context.Background(), &websearch.AnswerRequest{Query: "q"})
		requireTag(t, err, websearch.TagFeatureDisabled)
	})
	t.Run("invalid model", func(t *testing.T) {
		e := newEnv(t)
		_, err := e.provider(t).Answer(context.Background(), &websearch.AnswerRequest{Query: "q", Model: "gpt"})
		requireTag(t, err, websearch.TagInvalidRequest)
	})
	t.Run("search fails", func(t *testing.T) {
		e := newEnv(t)
		e.disc.EXPECT().Discover(gomock.Any(), gomock.Any()).Return(nil, errors.New("searxng down"))
		_, err := e.provider(t).Answer(context.Background(), &websearch.AnswerRequest{Query: "q"})
		require.ErrorContains(t, err, "searxng down")
	})
}

func TestStreamAnswer(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.discover(cands("https://a.com/")...)
	e.synth.EXPECT().Answer(gomock.Any(), gomock.Any(), gomock.Not(gomock.Nil())).DoAndReturn(
		func(_ context.Context, _ *AnswerSynthesis, onDelta func(string) error) (any, error) {
			require.NoError(t, onDelta("4"))
			require.NoError(t, onDelta("2"))
			return "42", nil
		})
	var events []websearch.AnswerEvent
	err := e.provider(t).StreamAnswer(context.Background(), &websearch.AnswerRequest{Query: "q", Stream: true},
		func(ev websearch.AnswerEvent) error {
			events = append(events, ev)
			return nil
		})
	require.NoError(t, err)
	require.Len(t, events, 4)
	assert.Equal(t, websearch.AnswerDelta{Role: "assistant", Content: "4"}, events[0].Choices[0].Delta)
	assert.Equal(t, websearch.AnswerDelta{Content: "2"}, events[1].Choices[0].Delta)
	assert.Equal(t, "https://a.com/", events[2].Citations[0].URL)
	assert.Nil(t, events[2].Citations[0].Text)
	assert.Empty(t, events[2].Choices)
	assert.True(t, strings.HasPrefix(events[3].RequestID, "req_"))
	assert.Nil(t, events[3].CostDollars)
	assert.Empty(t, events[3].Citations)
}

func TestStreamAnswerOutputSchema(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.discover(cands("https://a.com/")...)
	e.synth.EXPECT().Answer(gomock.Any(), gomock.Any(), gomock.Any()).Return(map[string]any{"x": 1}, nil)
	var events []websearch.AnswerEvent
	err := e.provider(t).StreamAnswer(context.Background(), &websearch.AnswerRequest{Query: "q", OutputSchema: json.RawMessage(`{"type":"object"}`)},
		func(ev websearch.AnswerEvent) error {
			events = append(events, ev)
			return nil
		})
	require.NoError(t, err)
	require.Len(t, events, 3)
	assert.Equal(t, `{"x":1}`, events[0].Choices[0].Delta.Content)
	assert.NotEmpty(t, events[1].Citations)
	assert.NotEmpty(t, events[2].RequestID)
}
