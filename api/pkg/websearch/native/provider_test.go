package native

import (
	"context"
	"net/http"
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
