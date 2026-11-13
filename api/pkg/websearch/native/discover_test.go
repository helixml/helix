package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/searxng"
	"github.com/helixml/helix/api/pkg/websearch"
)

var discoverNow = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

type fakeResult struct {
	URL           string  `json:"url"`
	Title         string  `json:"title"`
	Content       string  `json:"content,omitempty"`
	PublishedDate *string `json:"publishedDate"`
	Thumbnail     string  `json:"thumbnail,omitempty"`
}

// fakeSearXNG serves pages keyed by "q|pageno"; a missing key is an empty
// page, and a key in fail returns a 500. unresponsive lists engines reported
// as failed on every page.
type fakeSearXNG struct {
	mu           sync.Mutex
	pages        map[string][]fakeResult
	fail         map[string]bool
	unresponsive [][]string
	requests     []url.Values
}

func (f *fakeSearXNG) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := q.Get("pageno")
	if page == "" {
		page = "1"
	}
	key := q.Get("q") + "|" + page
	f.mu.Lock()
	f.requests = append(f.requests, q)
	results, failing, unresponsive := f.pages[key], f.fail[key], f.unresponsive
	f.mu.Unlock()
	if failing {
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	}
	if results == nil {
		results = []fakeResult{}
	}
	if unresponsive == nil {
		unresponsive = [][]string{}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results, "unresponsive_engines": unresponsive})
}

func (f *fakeSearXNG) queries() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]url.Values(nil), f.requests...)
}

func newTestDiscoverer(t *testing.T, f *fakeSearXNG) *SearXNGDiscoverer {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	d := NewSearXNGDiscoverer(searxng.NewClient(srv.URL, []string{"bing", "duckduckgo"}))
	d.now = func() time.Time { return discoverNow }
	return d
}

func res(u string) fakeResult { return fakeResult{URL: u, Title: u} }

func dated(u, date string) fakeResult {
	r := res(u)
	r.PublishedDate = &date
	return r
}

func urls(cs []Candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.URL
	}
	return out
}

func TestDiscover_TranslatesRequestToSearXNGParams(t *testing.T) {
	f := &fakeSearXNG{}
	d := newTestDiscoverer(t, f)
	start := discoverNow.Add(-3 * 24 * time.Hour)

	out, err := d.Discover(context.Background(), &DiscoverRequest{
		Queries:            []string{"chip export rules"},
		NumResults:         5,
		Category:           websearch.CategoryNews,
		IncludeDomains:     []string{"reuters.com", "*.ft.com", "apnews.com/hub", "reuters.com/world"},
		ExcludeDomains:     []string{"spam.com", "*.ads.net", "blog.example/drafts"},
		StartPublishedDate: &start,
		Country:            "de",
		Moderation:         true,
	})
	require.NoError(t, err)
	require.Empty(t, out.Candidates)
	require.Empty(t, out.Warnings)

	reqs := f.queries()
	require.Len(t, reqs, 1, "an empty first page stops paging")
	q := reqs[0]
	require.Equal(t, "chip export rules site:reuters.com OR site:ft.com OR site:apnews.com -site:spam.com", q.Get("q"),
		"path and subdomain-only excludes cannot be -site: operators")
	require.Equal(t, "news", q.Get("categories"))
	require.Empty(t, q.Get("engines"))
	require.Equal(t, "week", q.Get("time_range"))
	require.Equal(t, "de-DE", q.Get("language"))
	require.Equal(t, "2", q.Get("safesearch"))
}

func TestDiscover_TimeRangeIsSmallestCoveringStart(t *testing.T) {
	for age, want := range map[time.Duration]string{
		time.Hour:            "day",
		6 * 24 * time.Hour:   "week",
		20 * 24 * time.Hour:  "month",
		200 * 24 * time.Hour: "year",
		800 * 24 * time.Hour: "",
	} {
		f := &fakeSearXNG{}
		d := newTestDiscoverer(t, f)
		start := discoverNow.Add(-age)
		_, err := d.Discover(context.Background(), &DiscoverRequest{Queries: []string{"q"}, NumResults: 1, StartPublishedDate: &start})
		require.NoError(t, err)
		require.Equal(t, want, f.queries()[0].Get("time_range"), "age %s", age)
		require.Equal(t, "0", f.queries()[0].Get("safesearch"))
	}
}

func TestDiscover_PostFiltersDomainsWithSubdomainAndPathSemantics(t *testing.T) {
	f := &fakeSearXNG{pages: map[string][]fakeResult{
		"q site:example.com OR site:sub.org OR site:docs.io -site:bad.example.com|1": {
			res("https://www.example.com/a"),   // subdomain of example.com
			res("https://example.com/b"),       // exact host
			res("https://notexample.com/c"),    // suffix but not a subdomain
			res("https://bad.example.com/d"),   // excluded
			res("https://sub.org/e"),           // *.sub.org excludes the apex
			res("https://a.sub.org/f"),         // *.sub.org
			res("https://docs.io/guide"),       // path match
			res("https://docs.io/guide/intro"), // under the path
			res("https://docs.io/guides"),      // not a path segment match
			res("https://other.net/g"),
		},
	}}
	d := newTestDiscoverer(t, f)

	out, err := d.Discover(context.Background(), &DiscoverRequest{
		Queries:        []string{"q"},
		NumResults:     10,
		IncludeDomains: []string{"example.com", "*.sub.org", "https://docs.io/guide"},
		ExcludeDomains: []string{"bad.example.com"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		"https://www.example.com/a",
		"https://example.com/b",
		"https://a.sub.org/f",
		"https://docs.io/guide",
		"https://docs.io/guide/intro",
	}, urls(out.Candidates))
	require.Empty(t, out.Warnings)
}

func TestDiscover_DomainListsOverCapWarnAndStillFilter(t *testing.T) {
	var include, exclude []string
	for i := range 12 {
		include = append(include, fmt.Sprintf("in%d.com", i))
		exclude = append(exclude, fmt.Sprintf("out%d.com", i))
	}
	f := &fakeSearXNG{}
	d := newTestDiscoverer(t, f)

	out, err := d.Discover(context.Background(), &DiscoverRequest{
		Queries: []string{"q"}, NumResults: 3, IncludeDomains: include, ExcludeDomains: exclude,
	})
	require.NoError(t, err)

	want := "q"
	for i := range siteOperatorCap {
		want += fmt.Sprintf(" -site:out%d.com", i)
	}
	require.Equal(t, want, f.queries()[0].Get("q"), "includes are all-or-nothing, excludes are capped")
	require.Len(t, out.Warnings, 2)
	require.Equal(t, "includeDomains", out.Warnings[0].Path)
	require.Equal(t, "excludeDomains", out.Warnings[1].Path)
	for _, w := range out.Warnings {
		require.Equal(t, websearch.WarningApproximatedField, w.Code)
	}
}

func TestDiscover_DateFilterDropsOutOfRangeAndWarnsOnUndated(t *testing.T) {
	f := &fakeSearXNG{pages: map[string][]fakeResult{
		"q|1": {
			dated("https://a.com/old", "2026-01-01T00:00:00"),
			dated("https://a.com/in", "2026-11-01T09:30:00"),
			res("https://a.com/undated"),
			dated("https://a.com/future", "2026-11-12T00:00:00"),
		},
	}}
	d := newTestDiscoverer(t, f)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

	out, err := d.Discover(context.Background(), &DiscoverRequest{
		Queries: []string{"q"}, NumResults: 10, StartPublishedDate: &start, EndPublishedDate: &end,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"https://a.com/in", "https://a.com/undated"}, urls(out.Candidates))
	require.Equal(t, "year", f.queries()[0].Get("time_range"))
	require.Equal(t, []websearch.Warning{{
		Code: websearch.WarningApproximatedField, Path: "startPublishedDate",
		Message: "1 of 2 results have no known published date and were not date-filtered",
	}}, out.Warnings)
}

func TestDiscover_PagesUntilEnoughUniqueCandidates(t *testing.T) {
	f := &fakeSearXNG{pages: map[string][]fakeResult{
		"q|1": {res("https://a.com/1"), res("https://a.com/2")},
		"q|2": {res("https://a.com/2"), res("https://a.com/3"), res("https://skip.com/x")},
		"q|3": {res("https://a.com/4")},
	}}
	d := newTestDiscoverer(t, f)

	out, err := d.Discover(context.Background(), &DiscoverRequest{
		Queries: []string{"q"}, NumResults: 3, ExcludeDomains: []string{"*.skip.com", "skip.com/x"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"https://a.com/1", "https://a.com/2", "https://a.com/3"}, urls(out.Candidates))
	require.Len(t, f.queries(), 2, "page 3 is not needed")
}

func TestDiscover_StopsAtMaxPagesAndWhenPageRepeats(t *testing.T) {
	pages := map[string][]fakeResult{}
	for p := 1; p <= discoverMaxPages+2; p++ {
		pages["many|"+strconv.Itoa(p)] = []fakeResult{res(fmt.Sprintf("https://a.com/%d", p))}
	}
	pages["repeat|1"] = []fakeResult{res("https://r.com/1")}
	pages["repeat|2"] = []fakeResult{res("https://r.com/1#top")}
	f := &fakeSearXNG{pages: pages}
	d := newTestDiscoverer(t, f)

	out, err := d.Discover(context.Background(), &DiscoverRequest{Queries: []string{"many"}, NumResults: 50})
	require.NoError(t, err)
	require.Len(t, out.Candidates, discoverMaxPages)

	out, err = d.Discover(context.Background(), &DiscoverRequest{Queries: []string{"repeat"}, NumResults: 50})
	require.NoError(t, err)
	require.Len(t, out.Candidates, 1)
	require.Len(t, f.queries(), discoverMaxPages+2, "the repeated page ends paging")
}

func TestDiscover_MergesQueriesWithUserQueryDominant(t *testing.T) {
	f := &fakeSearXNG{pages: map[string][]fakeResult{
		"user|1": {res("https://u.com/1"), res("https://u.com/2"), res("https://shared.com/x")},
		"alt1|1": {res("https://a1.com/1"), res("https://SHARED.com/x#frag"), res("https://u.com/2")},
		"alt2|1": {res("https://a2.com/1"), dated("https://shared.com/x", "2026-11-01T00:00:00")},
	}}
	d := newTestDiscoverer(t, f)

	out, err := d.Discover(context.Background(), &DiscoverRequest{Queries: []string{"user", "alt1", "alt2"}, NumResults: 5})
	require.NoError(t, err)
	// shared.com/x is returned by all three; u.com/2 by two; u.com/1 leads
	// the user's query and outranks either other query's first result.
	require.Equal(t, []string{
		"https://shared.com/x",
		"https://u.com/2",
		"https://u.com/1",
		"https://a1.com/1",
		"https://a2.com/1",
	}, urls(out.Candidates))
	require.NotNil(t, out.Candidates[0].PublishedDate, "a missing date is filled from another occurrence")
	require.Empty(t, out.Warnings)
}

func TestDiscover_PartialFailureWarnsAndTotalFailureErrors(t *testing.T) {
	f := &fakeSearXNG{
		pages: map[string][]fakeResult{
			"ok|1": {res("https://a.com/1")},
			"ok|2": {res("https://a.com/2")},
		},
		fail: map[string]bool{"bad|1": true, "ok|3": true, "bad2|1": true},
	}
	d := newTestDiscoverer(t, f)

	out, err := d.Discover(context.Background(), &DiscoverRequest{Queries: []string{"ok", "bad"}, NumResults: 5})
	require.NoError(t, err)
	require.Equal(t, []string{"https://a.com/1", "https://a.com/2"}, urls(out.Candidates))
	require.Len(t, out.Warnings, 2)
	for _, w := range out.Warnings {
		require.Equal(t, websearch.WarningPartialResults, w.Code)
	}
	paths := []string{out.Warnings[0].Path, out.Warnings[1].Path}
	require.ElementsMatch(t, []string{"additionalQueries[0]", "query"}, paths, "a failed query, and a later page of one")

	_, err = d.Discover(context.Background(), &DiscoverRequest{Queries: []string{"bad", "bad2"}, NumResults: 5})
	var apiErr *websearch.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, websearch.TagUpstreamError, apiErr.Tag)
	require.Equal(t, http.StatusServiceUnavailable, apiErr.Status)
	require.NotContains(t, err.Error(), "status 500", "SearXNG's own errors stay in the server log")
	require.NotContains(t, err.Error(), "127.0.0.1", "SearXNG's address stays in the server log")
	for _, w := range out.Warnings {
		require.NotContains(t, w.Message, "status 500")
	}
}

func TestDiscover_WithoutCountrySearchesInQueryLanguage(t *testing.T) {
	f := &fakeSearXNG{}
	d := newTestDiscoverer(t, f)
	_, err := d.Discover(context.Background(), &DiscoverRequest{Queries: []string{"golang generics"}, NumResults: 1})
	require.NoError(t, err)
	require.Equal(t, "auto", f.queries()[0].Get("language"))
}

func TestDiscover_UnresponsiveEnginesWarn(t *testing.T) {
	f := &fakeSearXNG{
		pages:        map[string][]fakeResult{"q|1": {res("https://a.com/1")}},
		unresponsive: [][]string{{"duckduckgo", "CAPTCHA"}},
	}
	d := newTestDiscoverer(t, f)
	out, err := d.Discover(context.Background(), &DiscoverRequest{Queries: []string{"q"}, NumResults: 5})
	require.NoError(t, err)
	require.Equal(t, []string{"https://a.com/1"}, urls(out.Candidates))
	require.Equal(t, []websearch.Warning{{
		Code:    websearch.WarningPartialResults,
		Message: "results are from the other engines; these did not respond: duckduckgo (CAPTCHA)",
	}}, out.Warnings)
}

func TestDiscover_ApproximatedCategoryRewritesQuery(t *testing.T) {
	f := &fakeSearXNG{}
	d := newTestDiscoverer(t, f)

	out, err := d.Discover(context.Background(), &DiscoverRequest{
		Queries: []string{"jane doe"}, NumResults: 1, Category: websearch.CategoryPeople, Country: "XX",
	})
	require.NoError(t, err)
	q := f.queries()[0]
	require.Equal(t, "jane doe linkedin", q.Get("q"))
	require.Empty(t, q.Get("categories"))
	require.Equal(t, "bing,duckduckgo", q.Get("engines"))
	require.Equal(t, "auto", q.Get("language"), "an unknown country searches in the query's language")
	require.Len(t, out.Warnings, 2)
	require.Equal(t, websearch.Warning{
		Code: websearch.WarningApproximatedField, Path: "category",
		Message: `"people" is approximated by adding "linkedin" to a general web search`,
	}, out.Warnings[0])
	require.Equal(t, "userLocation.country", out.Warnings[1].Path)
}

func TestDiscover_RejectsInvalidInput(t *testing.T) {
	d := newTestDiscoverer(t, &fakeSearXNG{})
	for name, req := range map[string]*DiscoverRequest{
		"no queries":  {NumResults: 1},
		"no results":  {Queries: []string{"q"}},
		"category":    {Queries: []string{"q"}, NumResults: 1, Category: "research paper"},
		"bad domain":  {Queries: []string{"q"}, NumResults: 1, IncludeDomains: []string{"*"}},
		"bad exclude": {Queries: []string{"q"}, NumResults: 1, ExcludeDomains: []string{"a b.com"}},
	} {
		_, err := d.Discover(context.Background(), req)
		require.Error(t, err, name)
	}
}

// One engine's junk result costs only itself: a non-web URL is skipped, an
// unparseable date becomes no date, and later pages still count.
func TestDiscover_JunkResultsCostOnlyThemselves(t *testing.T) {
	bad := "13 Nov 2026"
	f := &fakeSearXNG{pages: map[string][]fakeResult{
		"golang|1": {res("https://go.dev/"), res("javascript:void(0)"), {URL: "https://x.example/", Title: "x", PublishedDate: &bad}},
		"golang|2": {res("ftp://mirror/"), res("https://pkg.go.dev/")},
	}}
	out, err := newTestDiscoverer(t, f).Discover(context.Background(), &DiscoverRequest{Queries: []string{"golang"}, NumResults: 3})
	require.NoError(t, err)
	require.Equal(t, []string{"https://go.dev/", "https://x.example/", "https://pkg.go.dev/"}, urls(out.Candidates))
	require.Nil(t, out.Candidates[1].PublishedDate)
	require.Empty(t, out.Warnings, "skipped junk is an operator concern, not a degraded result")
}

// The same page reached several ways is one candidate, kept under the URL
// the engines ranked first.
func TestDiscover_CollapsesNearDuplicateURLs(t *testing.T) {
	f := &fakeSearXNG{pages: map[string][]fakeResult{
		"golang|1": {
			res("https://go.dev/doc"), res("http://go.dev/doc"), res("https://www.go.dev/doc/"),
			res("https://go.dev/doc?utm_source=x&fbclid=y"), res("https://go.dev/doc?page=2"),
		},
	}}
	out, err := newTestDiscoverer(t, f).Discover(context.Background(), &DiscoverRequest{Queries: []string{"golang"}, NumResults: 10})
	require.NoError(t, err)
	require.Equal(t, []string{"https://go.dev/doc", "https://go.dev/doc?page=2"}, urls(out.Candidates))
}

func TestDiscover_CancelledRequestIsTheContextError(t *testing.T) {
	f := &fakeSearXNG{pages: map[string][]fakeResult{"golang|1": {res("https://go.dev/")}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newTestDiscoverer(t, f).Discover(ctx, &DiscoverRequest{Queries: []string{"golang"}, NumResults: 2})
	require.ErrorIs(t, err, context.Canceled)
	var apiErr *websearch.Error
	require.False(t, errors.As(err, &apiErr), "not reported as an upstream failure")
}

// A company query names the company; adding "company" pulls in pages about
// the word instead of its own site.
func TestDiscover_CompanyCategorySearchesTheQueryAsGiven(t *testing.T) {
	f := &fakeSearXNG{pages: map[string][]fakeResult{"Hugging Face|1": {res("https://huggingface.co/")}}}
	out, err := newTestDiscoverer(t, f).Discover(context.Background(), &DiscoverRequest{Queries: []string{"Hugging Face"}, NumResults: 1, Category: websearch.CategoryCompany})
	require.NoError(t, err)
	require.Equal(t, "Hugging Face", f.queries()[0].Get("q"))
	require.Equal(t, []string{"https://huggingface.co/"}, urls(out.Candidates))
	require.Len(t, out.Warnings, 1)
	require.Equal(t, websearch.WarningApproximatedField, out.Warnings[0].Code)
}
