package native

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/websearch"
)

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
	assert.Equal(t, "https://example.com/a", a.ID)
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
		{"summary without LLM", false, true, websearch.SearchRequest{Query: "q", Contents: &websearch.ContentsOptions{Summary: &websearch.SummaryOption{Enabled: true}}}, websearch.TagFeatureDisabled},
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
	assert.Equal(t, websearch.WarningPartialResults, resp.Warnings[0].Code)
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
	assert.Equal(t, websearch.WarningPartialResults, paths[""])
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

func TestSearchOutputSchemaWithoutSources(t *testing.T) {
	for _, stream := range []bool{false, true} {
		e := newEnv(t)
		e.pipeline(map[string]error{"https://a.com/": &FetchError{Tag: websearch.TagCrawlTimeout, Err: errors.New("slow")}})
		e.discover(cands("https://a.com/")...)
		// No Synthesize expectation: output grounded in nothing is never generated.
		req := &websearch.SearchRequest{Query: "q", OutputSchema: json.RawMessage(`{"type":"text"}`)}
		var err error
		if stream {
			err = e.provider(t).StreamSearch(context.Background(), req, func(websearch.SearchEvent) error { return nil })
		} else {
			_, err = e.provider(t).Search(context.Background(), req)
		}
		requireError(t, err, http.StatusBadGateway, websearch.TagUpstreamError)
	}
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
	require.Len(t, events[0].Warnings, 1, "Exa carries warnings on the results event")
	assert.Equal(t, "startCrawlDate", events[0].Warnings[0].Path)
	assert.Equal(t, "a", events[1].Delta)
	assert.Equal(t, out.Grounding, events[3].Grounding)
	done := events[4]
	assert.Equal(t, out, done.Output)
	assert.Empty(t, done.Warnings)
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
