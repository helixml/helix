package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/websearch"
)

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
	assert.Equal(t, "https://a.com/", resp.Results[0].ID)
	assert.Equal(t, "Page https://a.com/", resp.Results[0].Title)
	assert.Equal(t, "Ann", *resp.Results[0].Author)
	assert.Equal(t, []string{bodyOf("https://a.com/"), "cached " + bodyOf("https://d.com/")}, resultTexts(resp.Results))
	assert.Equal(t, []string{"https://a.com/"}, e.puts, "only fetched pages are written to the cache")
}

// A document id is the normalised URL search handed out, so an id resolves
// whether or not search fetched the page.
func TestContentsByID(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.cachePage("https://a.com/", time.Hour)

	resp, err := e.provider(t).Contents(context.Background(), &websearch.ContentsRequest{
		IDs: []string{"https://a.com/", "https://b.com/never-fetched"}, ContentsOptions: websearch.ContentsOptions{Text: &websearch.TextOption{Enabled: true}},
	})
	require.NoError(t, err)
	assert.Equal(t, []websearch.ContentStatus{
		{ID: "https://a.com/", Status: "success", Source: "cached"},
		{ID: "https://b.com/never-fetched", Status: "success", Source: "crawled"},
	}, resp.Statuses)
	require.Len(t, resp.Results, 2)
	assert.Equal(t, "https://a.com/", resp.Results[0].ID)
	assert.Equal(t, []string{"https://b.com/never-fetched"}, e.fetched)
}

func TestContentsRejectsNonURLID(t *testing.T) {
	e := newEnv(t)
	_, err := e.provider(t).Contents(context.Background(), &websearch.ContentsRequest{IDs: []string{"doc_1"}})
	require.Error(t, err)
	assert.Equal(t, websearch.TagInvalidRequest, websearch.AsError(err).Tag)
}

func TestContentsByIDRefetchesStalePage(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.cachePage("https://a.com/", 5*time.Hour)
	resp, err := e.provider(t).Contents(context.Background(), &websearch.ContentsRequest{
		IDs: []string{"https://a.com/"}, ContentsOptions: websearch.ContentsOptions{MaxAgeHours: ptr(1)},
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
	t.Run("a page the extractor cannot read is that page's status", func(t *testing.T) {
		e := newEnv(t)
		e.web(nil)
		e.memCache()
		e.ext.EXPECT().Extract(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, raw *RawPage, _ websearch.TextOptions) (*Page, error) {
				if raw.URL == "https://a.com/x.png" {
					return nil, fmt.Errorf("%w: image/png", ErrUnsupportedDocument)
				}
				return &Page{Text: string(raw.Body)}, nil
			}).Times(2)
		resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{}, "https://a.com/x.png", "https://b.com/"))
		require.NoError(t, err)
		assert.Equal(t, &websearch.ContentError{Tag: websearch.TagUnsupportedDocument}, resp.Statuses[0].Error)
		assert.Equal(t, websearch.ContentStatusSuccess, resp.Statuses[1].Status)
		assert.Equal(t, []string{"https://b.com/"}, resultURLs(resp.Results))
	})
	t.Run("the document service failing on a page is that page's status", func(t *testing.T) {
		e := newEnv(t)
		e.web(nil)
		e.memCache()
		e.ext.EXPECT().Extract(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, raw *RawPage, _ websearch.TextOptions) (*Page, error) {
				if raw.URL == "https://a.com/paper.pdf" {
					return nil, fmt.Errorf("%w: llamaindex: no such host", ErrExtractionFailed)
				}
				return &Page{Text: string(raw.Body)}, nil
			}).Times(2)
		resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{}, "https://a.com/paper.pdf", "https://b.com/"))
		require.NoError(t, err)
		assert.Equal(t, &websearch.ContentError{Tag: websearch.TagExtractionFailed}, resp.Statuses[0].Error)
		assert.Equal(t, []string{"https://b.com/"}, resultURLs(resp.Results))
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
		{"summary without LLM", false, true, contentsReq(websearch.ContentsOptions{Summary: &websearch.SummaryOption{Enabled: true}}, "https://a.com"), websearch.TagFeatureDisabled},
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
	e := newEnv(t).withRanker()
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
	e := newEnv(t).withRanker()
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
		Summary: &websearch.SummaryOption{Enabled: true, SummaryOptions: websearch.SummaryOptions{Query: "focus", Schema: schema}}}, "https://a.com/"))
	require.NoError(t, err)
	assert.Equal(t, `{"a":1}`, *resp.Results[0].Summary)
}

func TestContentsSummarySkipsEmptyPage(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.bodies["https://a.com/"] = " \n"
	resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Summary: &websearch.SummaryOption{Enabled: true}}, "https://a.com/"))
	require.NoError(t, err, "an empty page does not fail the request")
	assert.Nil(t, resp.Results[0].Summary)
}

// An option set to false is not requested: it needs no LLM or ranker and
// returns nothing.
func TestContentsOptionsSetToFalse(t *testing.T) {
	e := newEnv(t)
	e.opts.Ranker, e.opts.Synthesizer = nil, nil
	e.pipeline(nil)
	resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Text:       &websearch.TextOption{},
		Highlights: &websearch.HighlightsOption{HighlightsOptions: websearch.HighlightsOptions{HighlightsPerURL: 2}},
		Summary:    &websearch.SummaryOption{},
		Context:    &websearch.ContextOption{},
	}, "https://a.com/"))
	require.NoError(t, err)
	r := resp.Results[0]
	assert.Nil(t, r.Text)
	assert.Nil(t, r.Highlights)
	assert.Nil(t, r.Summary)
	assert.Empty(t, resp.Context)
	assert.Empty(t, resp.Warnings, "options of an unrequested field are not reported")
}

func TestContentsWarnsAboutDeprecatedFields(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Extras: &websearch.ExtrasOptions{RichLinks: 1, CodeBlocks: 1},
	}, "https://a.com/"))
	require.NoError(t, err)
	assert.Equal(t, []websearch.Warning{
		{Code: websearch.WarningIgnoredField, Path: "extras.richLinks", Message: "extras.richLinks is deprecated and ignored"},
		{Code: websearch.WarningIgnoredField, Path: "extras.codeBlocks", Message: "extras.codeBlocks is deprecated and ignored"},
	}, resp.Warnings)
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
	assert.Equal(t, "https://a.com/docs", sub[1].ID)
	assert.Nil(t, sub[0].Subpages)
	assert.Len(t, resp.Statuses, 1, "statuses cover the requested pages only")
	assert.Equal(t, []websearch.Warning{{Code: websearch.WarningPartialResults, Message: "1 of 4 subpages could not be fetched"}}, resp.Warnings)
}

// Links are relative to where the page landed, so subpages follow a redirect.
func TestContentsSubpagesAfterRedirect(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	e.finals["https://a.com/"] = "https://www.a.com/"
	e.pages["https://a.com/"] = &Page{Title: "Home", Links: []Link{
		{URL: "https://www.a.com/"}, // the landing page itself
		{URL: "https://www.a.com/docs"},
	}}
	resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{Subpages: 2}, "https://a.com/"))
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, []string{"https://www.a.com/docs"}, resultURLs(resp.Results[0].Subpages))
}

func TestContentsContext(t *testing.T) {
	e := newEnv(t)
	e.pipeline(nil)
	resp, err := e.provider(t).Contents(context.Background(), contentsReq(websearch.ContentsOptions{
		Context: &websearch.ContextOption{Enabled: true}}, "https://a.com/"))
	require.NoError(t, err)
	assert.Equal(t, "Title: Page https://a.com/\nURL: https://a.com/\ntext of https://a.com/", resp.Context)
}
