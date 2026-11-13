package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/netguard"
	helixopenai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/searxng"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/websearch"
	"github.com/helixml/helix/api/pkg/websearch/native"
)

// WebSearchIntegrationSuite runs the Exa-compatible API end to end: the real
// handlers over HTTP, the real native provider and its real components
// (SearXNG client and discoverer, guarded fetcher, extractor, embedding
// ranker, LLM synthesizer, OpenAI client), with only the network edges
// faked (websearch_integration_fakes_test.go). Every request sent and every
// response returned is checked against Exa's published OpenAPI spec
// (websearch_exa_spec_test.go).
//
// It lives in package server because the handlers are unexported methods
// of HelixAPIServer; it needs no database, auth or CGo.
type WebSearchIntegrationSuite struct {
	suite.Suite
	spec      *exaSpec
	web       *wsWeb
	searx     *wsSearXNG
	llm       *wsLLM
	extractor *wsExtractor
	cache     *wsPageCache
	// api serves the full stack (ranker and synthesizer configured) with
	// private networks allowed, since the fake web is on loopback.
	api *httptest.Server
}

func TestWebSearchIntegrationSuite(t *testing.T) {
	suite.Run(t, new(WebSearchIntegrationSuite))
}

const (
	wsLLMProvider = "fake-llm"
	wsUserID      = "usr_websearch_it"
	wsOrgID       = "org_websearch_it"
)

type wsStack struct {
	policy netguard.Policy
	cache  *wsPageCache
	llm    bool // configure the ranker and synthesizer
}

func (s *WebSearchIntegrationSuite) SetupTest() {
	t := s.T()
	s.spec = loadExaSpec(t)
	s.web = newWSWeb(t)
	s.searx = newWSSearXNG(t, s.web)
	s.llm = newWSLLM(t)
	s.extractor = newWSExtractor(t)
	s.cache = newWSPageCache()
	s.api = s.startAPI(wsStack{policy: netguard.Policy{AllowPrivate: true}, cache: s.cache, llm: true})
}

// startAPI assembles a native provider the way setup.NewProvider does and
// serves the web search routes over HTTP as an authenticated user.
func (s *WebSearchIntegrationSuite) startAPI(stack wsStack) *httptest.Server {
	t := s.T()
	fetcher, err := native.NewFetcher(native.FetcherOptions{Policy: stack.policy})
	s.Require().NoError(err)
	opts := native.Options{
		Discoverer:       native.NewSearXNGDiscoverer(searxng.NewClient(s.searx.srv.URL, []string{"duckduckgo", "brave"})),
		Fetcher:          fetcher,
		Extractor:        native.NewExtractor(extract.NewDefaultExtractor(s.extractor.srv.URL)),
		Cache:            stack.cache,
		FetchConcurrency: 4,
	}
	if stack.llm {
		client := helixopenai.New(wsLLMAPIKey, s.llm.baseURL(), false)
		providers := manager.NewMockProviderManager(gomock.NewController(t))
		providers.EXPECT().GetClient(gomock.Any(), gomock.Any()).AnyTimes().
			DoAndReturn(func(_ context.Context, req *manager.GetClientRequest) (helixopenai.Client, error) {
				if req.Provider != wsLLMProvider || req.Owner != wsUserID {
					return nil, fmt.Errorf("unexpected client request: provider %q owner %q", req.Provider, req.Owner)
				}
				return client, nil
			})
		opts.Ranker = native.NewEmbeddingRanker(providers, wsLLMProvider, wsEmbeddingModel)
		opts.Synthesizer, err = native.NewLLMSynthesizer(providers, wsLLMProvider, map[native.Tier]string{
			native.TierFast: wsFastModel, native.TierDefault: wsDefaultModel, native.TierStrong: wsStrongModel,
		})
		s.Require().NoError(err)
	}
	provider, err := native.New(opts)
	s.Require().NoError(err)

	apiServer := &HelixAPIServer{webSearch: provider}
	router := mux.NewRouter()
	apiServer.registerWebSearchRoutes(router.PathPrefix(APIPrefix).Subrouter())
	handler := cleanWebSearchBasePath(router)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := setRequestUser(r.Context(), types.User{ID: wsUserID, OrganizationID: wsOrgID})
		handler.ServeHTTP(rw, r.WithContext(ctx))
	}))
	t.Cleanup(srv.Close)
	return srv
}

type wsResponse struct {
	status int
	header http.Header
	body   []byte
	// events are the data payloads of an SSE response, without [DONE].
	events []json.RawMessage
}

func (s *WebSearchIntegrationSuite) post(api *httptest.Server, path, body string) wsResponse {
	req, err := http.NewRequest(http.MethodPost, api.URL+webSearchPrefix+path, strings.NewReader(body))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := api.Client().Do(req)
	s.Require().NoError(err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	s.Require().NoError(err)
	return wsResponse{status: resp.StatusCode, header: resp.Header, body: data}
}

// call sends a request a real Exa client could send and checks the
// response against the spec.
func (s *WebSearchIntegrationSuite) call(api *httptest.Server, path, body string) wsResponse {
	s.Require().NoError(s.spec.validateRequest(path, []byte(body)), "test request must be valid per Exa's spec: %s", body)
	return s.send(api, path, body)
}

// send posts body without checking it against the spec, and checks the
// response: every JSON body and SSE event against the spec, and the
// x-request-id header against the body's requestId.
func (s *WebSearchIntegrationSuite) send(api *httptest.Server, path, body string) wsResponse {
	resp := s.post(api, path, body)
	requestID := resp.header.Get("x-request-id")
	s.NotEmpty(requestID, "x-request-id header")
	if strings.HasPrefix(resp.header.Get("Content-Type"), "text/event-stream") {
		resp.events = s.parseEvents(path, resp)
		return resp
	}
	s.NoError(s.spec.validateJSONResponse(path, []byte(body), resp.status, resp.header, resp.body),
		"%s %d response does not conform to Exa's spec:\n%s", path, resp.status, resp.body)
	var envelope struct {
		RequestID string `json:"requestId"`
	}
	s.Require().NoError(json.Unmarshal(resp.body, &envelope), "%s", resp.body)
	s.Equal(requestID, envelope.RequestID, "body requestId matches x-request-id")
	return resp
}

// parseEvents parses an SSE body, checks it ends with [DONE] and that every
// other event conforms to the spec, and returns the events.
func (s *WebSearchIntegrationSuite) parseEvents(path string, resp wsResponse) []json.RawMessage {
	s.Require().Equal(http.StatusOK, resp.status)
	chunks := strings.Split(strings.TrimSuffix(string(resp.body), "\n\n"), "\n\n")
	s.Require().NotEmpty(chunks)
	s.Equal("data: [DONE]", chunks[len(chunks)-1], "stream ends with [DONE]")
	var out []json.RawMessage
	for _, c := range chunks[:len(chunks)-1] {
		data, ok := strings.CutPrefix(c, "data: ")
		s.Require().True(ok, "SSE chunk %q", c)
		var event any
		s.Require().NoError(json.Unmarshal([]byte(data), &event), "event %s", data)
		s.NoError(s.spec.validateStreamEvent(path, event), "%s stream event does not conform to Exa's spec:\n%s", path, data)
		out = append(out, json.RawMessage(data))
	}
	return out
}

func decodeAs[T any](s *WebSearchIntegrationSuite, resp wsResponse) T {
	var v T
	s.Require().NoError(json.Unmarshal(resp.body, &v), "%s", resp.body)
	return v
}

func (s *WebSearchIntegrationSuite) requireOK(resp wsResponse) {
	s.Require().Equal(http.StatusOK, resp.status, "%s", resp.body)
}

func (s *WebSearchIntegrationSuite) requireError(resp wsResponse, status int, tag string) websearch.ErrorResponse {
	s.Require().Equal(status, resp.status, "%s", resp.body)
	s.Equal("application/json", resp.header.Get("Content-Type"))
	body := decodeAs[websearch.ErrorResponse](s, resp)
	s.Equal(tag, body.Tag, "%s", resp.body)
	return body
}

func jsonBody(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func resultURLs(results []websearch.Result) []string {
	urls := make([]string, len(results))
	for i, r := range results {
		urls[i] = r.URL
	}
	return urls
}

// firstPageQueries are the q values of the first-page SearXNG requests.
func (s *WebSearchIntegrationSuite) firstPageQueries() []string {
	var qs []string
	for _, q := range s.searx.requests() {
		if q.Get("pageno") == "" {
			qs = append(qs, q.Get("q"))
		}
	}
	return qs
}

func (s *WebSearchIntegrationSuite) genericsURL() string {
	return s.web.newsURL("/articles/go-generics")
}

func (s *WebSearchIntegrationSuite) rustURL() string {
	return s.web.blogURL("/posts/rust-vs-go")
}

// --- /search ---

func (s *WebSearchIntegrationSuite) TestSearchPlain() {
	resp := s.call(s.api, "/search", `{"query":"go generics","numResults":3}`)
	s.requireOK(resp)
	body := decodeAs[websearch.SearchResponse](s, resp)

	s.Equal(websearch.SearchTypeAuto, body.ResolvedSearchType)
	s.Equal([]string{s.genericsURL(), s.rustURL(), s.web.newsURL("/articles/go-errors")}, resultURLs(body.Results),
		"SearXNG order, URLs normalised (host lower-cased, fragment dropped)")
	for _, r := range body.Results {
		s.Equal(r.URL, r.ID, "the document id is the normalised URL")
		s.Nil(r.Text, "no contents requested")
		s.Require().NotNil(r.PublishedDate, "%s: SearXNG's publishedDate", r.URL)
	}
	s.WithinDuration(s.searx.now.Add(-10*24*time.Hour), *body.Results[0].PublishedDate, time.Second, "naive datetime with microseconds, read as UTC")
	s.Equal(s.searx.now.Add(-60*24*time.Hour).Format("2006-01-02"), body.Results[1].PublishedDate.UTC().Format("2006-01-02"), "date-only")
	s.Empty(body.Warnings)
	s.Zero(s.web.hitCount(s.genericsURL()), "a search without contents fetches nothing")

	reqs := s.searx.requests()
	s.Require().NotEmpty(reqs)
	s.Equal("go generics", reqs[0].Get("q"))
	s.Equal("duckduckgo,brave", reqs[0].Get("engines"))
	s.Equal(searxng.LanguageAuto, reqs[0].Get("language"))
	s.Equal("0", reqs[0].Get("safesearch"))
}

func (s *WebSearchIntegrationSuite) TestSearchWithContents() {
	resp := s.call(s.api, "/search", jsonBody(map[string]any{
		"query":      "go generics",
		"numResults": 2,
		"contents": map[string]any{
			"text":          map[string]any{"maxCharacters": 4000},
			"highlights":    map[string]any{"query": "type parameters and constraints", "maxCharacters": 700},
			"summary":       map[string]any{"query": "what changed in Go"},
			"extras":        map[string]any{"links": 20, "imageLinks": 2},
			"subpages":      1,
			"subpageTarget": "constraints",
		},
	}))
	s.requireOK(resp)
	body := decodeAs[websearch.SearchResponse](s, resp)
	s.Require().Equal([]string{s.genericsURL(), s.rustURL()}, resultURLs(body.Results))
	s.Empty(body.Warnings)

	generics := body.Results[0]
	s.Require().NotNil(generics.Text)
	s.Contains(*generics.Text, "Generic functions and types take type parameters")
	s.NotContains(*generics.Text, "Copyright 2026 Example News", "footer is not main content")
	s.NotContains(*generics.Text, "We use cookies", "banner is not main content")
	s.Require().NotNil(generics.Author)
	s.Equal("Ada Lovelace", *generics.Author)
	s.Equal(s.web.newsURL("/img/generics.png"), generics.Image)
	s.Equal(s.web.newsURL("/favicon.ico"), generics.Favicon)
	s.Require().NotEmpty(generics.Highlights)
	for _, h := range generics.Highlights {
		s.Contains(*generics.Text, h, "a highlight is a verbatim passage of the page")
	}
	s.Contains(strings.Join(generics.Highlights, " "), "type parameter")
	s.Require().NotNil(generics.Summary)
	s.Equal(wsSummaryReply, *generics.Summary)
	s.Require().NotNil(generics.Extras)
	s.Contains(generics.Extras.Links, s.web.newsURL("/articles/go-generics/part-2"))
	s.Contains(generics.Extras.ImageLinks, s.web.newsURL("/img/diagram.png"))
	s.Require().Len(generics.Subpages, 1, "subpages: 1")
	sub := generics.Subpages[0]
	s.Equal(s.web.newsURL("/articles/go-generics/part-2"), sub.URL, "subpageTarget picks the link whose anchor mentions constraints")
	s.Equal(sub.URL, sub.ID)
	s.Contains(sub.Title, "constraints")
	s.Require().NotNil(sub.Text)
	s.Contains(*sub.Text, "comparable")

	rust := body.Results[1]
	s.Require().NotNil(rust.Author)
	s.Equal("Grace Hopper", *rust.Author, "author from JSON-LD")
	s.Require().NotNil(rust.Text)
	s.Contains(*rust.Text, "monomorphizes")
	s.Require().Len(rust.Subpages, 1)
	s.Equal(s.web.blogURL("/"), rust.Subpages[0].URL)

	var embeddings, summaries int
	for _, c := range s.llm.recorded() {
		switch {
		case c.Path == "embeddings":
			embeddings++
		case c.Model == wsFastModel && c.SchemaName == "":
			summaries++
		}
	}
	s.Positive(embeddings, "highlights are ranked by embeddings")
	s.Equal(4, summaries, "one summary per result and subpage, on the fast model")
}

func (s *WebSearchIntegrationSuite) TestSearchFilters() {
	day := 24 * time.Hour
	s.Run("includeDomains", func() {
		resp := s.call(s.api, "/search", `{"query":"go generics","includeDomains":["127.0.0.1"]}`)
		s.requireOK(resp)
		body := decodeAs[websearch.SearchResponse](s, resp)
		s.Require().NotEmpty(body.Results)
		for _, r := range body.Results {
			s.True(strings.HasPrefix(r.URL, s.web.news.URL), r.URL)
		}
		s.Contains(s.firstPageQueries(), "go generics site:127.0.0.1")
	})
	s.Run("excludeDomains", func() {
		resp := s.call(s.api, "/search", `{"query":"go errors","excludeDomains":["localhost"]}`)
		s.requireOK(resp)
		body := decodeAs[websearch.SearchResponse](s, resp)
		s.Require().NotEmpty(body.Results)
		for _, r := range body.Results {
			s.NotContains(r.URL, "localhost")
		}
		s.Contains(s.firstPageQueries(), "go errors -site:localhost")
	})
	s.Run("startPublishedDate", func() {
		resp := s.call(s.api, "/search", jsonBody(map[string]any{
			"query": "go dates", "startPublishedDate": s.searx.now.Add(-90 * day).Format(time.RFC3339),
		}))
		s.requireOK(resp)
		body := decodeAs[websearch.SearchResponse](s, resp)
		s.Equal([]string{s.genericsURL(), s.rustURL(), s.web.blogURL("/posts/go-concurrency")}, resultURLs(body.Results),
			"the 400-day-old article is filtered out; the undated post is kept")
		s.Require().Len(body.Warnings, 1)
		s.Equal(websearch.WarningApproximatedField, body.Warnings[0].Code)
		s.Equal("/startPublishedDate", body.Warnings[0].Path)
		var sent string
		for _, q := range s.searx.requests() {
			if q.Get("q") == "go dates" {
				sent = q.Get("time_range")
			}
		}
		s.Equal(searxng.TimeRangeYear, sent)
	})
	s.Run("endPublishedDate", func() {
		resp := s.call(s.api, "/search", jsonBody(map[string]any{
			"query": "go dates", "endPublishedDate": s.searx.now.Add(-30 * day).Format(time.RFC3339),
		}))
		s.requireOK(resp)
		body := decodeAs[websearch.SearchResponse](s, resp)
		s.Equal([]string{s.rustURL(), s.web.newsURL("/articles/go-errors"), s.web.blogURL("/posts/go-concurrency")}, resultURLs(body.Results))
	})
}

func (s *WebSearchIntegrationSuite) TestSearchNewsCategory() {
	resp := s.call(s.api, "/search", `{"query":"go release","category":"news"}`)
	s.requireOK(resp)
	body := decodeAs[websearch.SearchResponse](s, resp)
	s.Equal([]string{s.genericsURL(), s.web.newsURL("/articles/go-errors")}, resultURLs(body.Results))

	reqs := s.searx.requests()
	s.Require().NotEmpty(reqs)
	s.Equal("news", reqs[0].Get("categories"))
	s.Empty(reqs[0].Get("engines"), "categories replace the configured engines")
}

func (s *WebSearchIntegrationSuite) TestDeepSearchWithAdditionalQueries() {
	resp := s.call(s.api, "/search", `{"query":"go generics","type":"deep","additionalQueries":["go type parameters"],"numResults":3}`)
	s.requireOK(resp)
	body := decodeAs[websearch.SearchResponse](s, resp)
	s.Equal(websearch.SearchTypeDeep, body.ResolvedSearchType)
	s.Len(body.Results, 3)
	s.ElementsMatch([]string{"go generics", "go type parameters", "golang type parameters tutorial", "go generic constraints"},
		s.firstPageQueries(), "the query, additionalQueries and the LLM's expansions, deduplicated")

	var expansions []wsLLMCall
	for _, c := range s.llm.recorded() {
		if c.SchemaName == "queries" {
			expansions = append(expansions, c)
		}
	}
	s.Require().Len(expansions, 1)
	s.Equal(wsFastModel, expansions[0].Model)
}

func (s *WebSearchIntegrationSuite) TestSearchOutputSchemaObject() {
	resp := s.call(s.api, "/search", jsonBody(map[string]any{
		"query": "when did go get generics",
		"outputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"release":  map[string]any{"type": "string", "description": "Go version that added generics"},
				"features": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"release"},
		},
	}))
	s.requireOK(resp)
	body := decodeAs[websearch.SearchResponse](s, resp)
	s.Require().NotNil(body.Output)
	content, ok := body.Output.Content.(map[string]any)
	s.Require().True(ok, "object schema → object content: %#v", body.Output.Content)
	s.Equal("Go 1.18 (release)", content["release"])
	s.Equal([]any{"Go 1.18 (features)"}, content["features"])
	s.Require().Len(body.Output.Grounding, 2)
	for _, g := range body.Output.Grounding {
		s.Contains([]string{"features", "release"}, g.Field)
		s.Equal("high", g.Confidence)
		s.Equal([]string{s.genericsURL(), s.rustURL()}, []string{g.Citations[0].URL, g.Citations[1].URL},
			"source numbers map back to the fetched results")
	}
	var synth []wsLLMCall
	for _, c := range s.llm.recorded() {
		if c.SchemaName == "search_output" {
			synth = append(synth, c)
		}
	}
	s.Require().Len(synth, 1)
	s.Equal(wsDefaultModel, synth[0].Model)
}

func (s *WebSearchIntegrationSuite) TestSearchOutputSchemaText() {
	resp := s.call(s.api, "/search", `{"query":"when did go get generics","outputSchema":{"type":"text","description":"two sentences"}}`)
	s.requireOK(resp)
	body := decodeAs[websearch.SearchResponse](s, resp)
	s.Require().NotNil(body.Output)
	s.Equal(wsSynthesisReply, body.Output.Content)
	s.Require().Len(body.Output.Grounding, 1)
	g := body.Output.Grounding[0]
	s.Equal("content", g.Field)
	s.Equal("high", g.Confidence, "two distinct sources cited")
	s.Equal([]websearch.Citation{
		{URL: s.genericsURL(), Title: "Go 1.18 brings generics to the language"},
		{URL: s.rustURL(), Title: "Rust traits versus Go generics"},
	}, g.Citations)
}

func (s *WebSearchIntegrationSuite) TestStreamSearch() {
	resp := s.call(s.api, "/search", `{"query":"when did go get generics","stream":true,"outputSchema":{"type":"text"}}`)
	s.requireOK(resp)
	s.Equal("text/event-stream", resp.header.Get("Content-Type"))
	events := resp.events

	var kinds []string
	var text strings.Builder
	var done websearch.SearchEvent
	for _, raw := range events {
		var ev websearch.SearchEvent
		s.Require().NoError(json.Unmarshal(raw, &ev))
		if len(kinds) == 0 || kinds[len(kinds)-1] != ev.Type {
			kinds = append(kinds, ev.Type)
		}
		switch ev.Type {
		case websearch.SearchEventResults:
			s.Len(ev.Results, 4)
		case websearch.SearchEventTextDelta:
			s.Require().NotNil(ev.Delta, "text-delta must carry delta")
			text.WriteString(*ev.Delta)
		case websearch.SearchEventDone:
			done = ev
		}
	}
	s.Equal([]string{"results", "text-delta", "grounding", "done"}, kinds)
	s.Equal(wsSynthesisReply, text.String(), "the deltas are the streamed LLM output")
	s.Require().NotNil(done.Output)
	s.Equal(wsSynthesisReply, done.Output.Content)
	s.Positive(done.SearchTime)
}

// TestStreamSearchWarnings: Exa sends warnings on the results chunk; its
// done chunk has no warnings property and allows no others.
func (s *WebSearchIntegrationSuite) TestStreamSearchWarnings() {
	resp := s.call(s.api, "/search", `{"query":"when did go get generics","stream":true,"outputSchema":{"type":"text"},"startCrawlDate":"2026-01-01T00:00:00Z"}`)
	s.requireOK(resp)
	events := resp.events
	var resultsWarnings []websearch.Warning
	for _, raw := range events {
		var ev websearch.SearchEvent
		s.Require().NoError(json.Unmarshal(raw, &ev))
		if ev.Type == websearch.SearchEventResults {
			resultsWarnings = ev.Warnings
		}
	}
	s.Require().Len(resultsWarnings, 1, "the ignored startCrawlDate is reported on the results chunk")
	s.Equal(websearch.WarningIgnoredField, resultsWarnings[0].Code)
}

// TestStreamSearchFailure: an LLM failure after streaming started ends the
// stream with an error event, then [DONE].
func (s *WebSearchIntegrationSuite) TestStreamSearchFailure() {
	resp := s.call(s.api, "/search", `{"query":"go generics interrupted","stream":true,"outputSchema":{"type":"text"}}`)
	s.requireOK(resp)
	s.Require().NotEmpty(resp.events)
	var last websearch.SearchEvent
	s.Require().NoError(json.Unmarshal(resp.events[len(resp.events)-1], &last))
	s.Equal(websearch.SearchEventError, last.Type)
	s.Require().NotNil(last.Error)
	s.Contains(last.Error.Message, "upstream model overloaded")
}

// TestSearchEngineOutageWarning: one engine timing out while another answers
// is reported as a warning, and that warning must fit Exa's schema.
func (s *WebSearchIntegrationSuite) TestSearchEngineOutageWarning() {
	resp := s.call(s.api, "/search", `{"query":"go generics flaky"}`)
	s.requireOK(resp)
	body := decodeAs[websearch.SearchResponse](s, resp)
	s.NotEmpty(body.Results)
	s.Require().Len(body.Warnings, 1)
	s.Equal(websearch.WarningPartialResults, body.Warnings[0].Code)
	s.Contains(body.Warnings[0].Message, "brave (timeout)")
}

// --- /contents ---

func (s *WebSearchIntegrationSuite) TestContentsByURL() {
	pdf := s.web.newsURL("/report.pdf")
	resp := s.call(s.api, "/contents", jsonBody(map[string]any{
		"urls":   []string{s.genericsURL(), s.rustURL(), pdf},
		"text":   true,
		"extras": map[string]any{"richImageLinks": 1},
	}))
	s.requireOK(resp)
	body := decodeAs[websearch.ContentsResponse](s, resp)

	s.Require().Len(body.Statuses, 3)
	for i, id := range []string{s.genericsURL(), s.rustURL(), pdf} {
		s.Equal(websearch.ContentStatus{ID: id, Status: websearch.ContentStatusSuccess, Source: websearch.ContentSourceCrawled}, body.Statuses[i])
	}
	s.Require().Equal([]string{s.genericsURL(), s.rustURL(), pdf}, resultURLs(body.Results))
	generics := body.Results[0]
	s.Equal("Go 1.18 brings generics to the language", generics.Title)
	s.Require().NotNil(generics.Text)
	s.Contains(*generics.Text, "Ian Lance Taylor")
	s.NotContains(*generics.Text, "Most read", "sidebar is not main content")
	s.Require().NotNil(generics.Extras)
	s.Equal([]websearch.ImageLink{{URL: s.web.newsURL("/img/diagram.png"), Alt: "Type parameter diagram"}}, generics.Extras.RichImageLinks)
	s.Require().NotNil(generics.PublishedDate)
	s.Equal("2026-10-03T09:30:00Z", generics.PublishedDate.UTC().Format(time.RFC3339), "article:published_time")

	report := body.Results[2]
	s.Equal("Go Developer Survey 2026", report.Title, "a PDF is titled by its first line")
	s.Require().NotNil(report.Text)
	s.Equal(wsPDFText, *report.Text, "PDF text from the extraction service")
}

func (s *WebSearchIntegrationSuite) TestContentsMixedOutcomes() {
	targets := []string{
		s.genericsURL(),
		s.web.newsURL("/missing"),
		s.web.newsURL("/slow"),
		"http://169.254.169.254/latest/meta-data/", // cloud metadata: blocked under every policy
	}
	start := time.Now()
	resp := s.call(s.api, "/contents", jsonBody(map[string]any{"urls": targets, "text": true, "livecrawlTimeout": 500}))
	s.requireOK(resp)
	s.Less(time.Since(start), 5*time.Second, "livecrawlTimeout bounds the slow page")
	body := decodeAs[websearch.ContentsResponse](s, resp)

	s.Equal([]websearch.ContentStatus{
		{ID: targets[0], Status: websearch.ContentStatusSuccess, Source: websearch.ContentSourceCrawled},
		{ID: targets[1], Status: websearch.ContentStatusError, Error: &websearch.ContentError{Tag: websearch.TagCrawlNotFound, HTTPStatusCode: http.StatusNotFound}},
		{ID: targets[2], Status: websearch.ContentStatusError, Error: &websearch.ContentError{Tag: websearch.TagCrawlTimeout}},
		{ID: targets[3], Status: websearch.ContentStatusError, Error: &websearch.ContentError{Tag: websearch.TagBlockedDestination}},
	}, body.Statuses)
	s.Equal([]string{s.genericsURL()}, resultURLs(body.Results), "only successes are results")
}

func (s *WebSearchIntegrationSuite) TestContentsBlockedByDefaultPolicy() {
	api := s.startAPI(wsStack{policy: netguard.Policy{}, cache: newWSPageCache(), llm: true})
	targets := []string{s.genericsURL(), s.rustURL()}
	resp := s.call(api, "/contents", jsonBody(map[string]any{"urls": targets, "text": true}))
	s.requireOK(resp)
	body := decodeAs[websearch.ContentsResponse](s, resp)
	for i, st := range body.Statuses {
		s.Equal(targets[i], st.ID)
		s.Equal(websearch.ContentStatusError, st.Status)
		s.Require().NotNil(st.Error)
		s.Equal(websearch.TagBlockedDestination, st.Error.Tag, "loopback is a private address")
	}
	s.Empty(body.Results)
	s.Zero(s.web.hitCount(s.genericsURL()))
	s.Zero(s.web.hitCount(s.rustURL()))
}

func (s *WebSearchIntegrationSuite) TestContentsByIDsAndCache() {
	search := decodeAs[websearch.SearchResponse](s, s.call(s.api, "/search", `{"query":"go generics","numResults":2}`))
	s.Require().Len(search.Results, 2)
	ids := []string{search.Results[0].ID, search.Results[1].ID}

	statuses := func(body string) []websearch.ContentStatus {
		resp := s.call(s.api, "/contents", body)
		s.requireOK(resp)
		out := decodeAs[websearch.ContentsResponse](s, resp)
		for i, r := range out.Results {
			s.Equal(out.Statuses[i].ID, r.ID, "the result id is the requested id")
		}
		return out.Statuses
	}
	sources := func(sts []websearch.ContentStatus) []string {
		var out []string
		for _, st := range sts {
			s.Equal(websearch.ContentStatusSuccess, st.Status, "%+v", st)
			out = append(out, st.Source)
		}
		return out
	}

	s.Equal([]string{"crawled", "crawled"}, sources(statuses(jsonBody(map[string]any{"ids": ids, "text": true}))), "ids from a search resolve to their pages")
	s.Equal(1, s.web.hitCount(s.genericsURL()))

	s.Equal([]string{"cached", "cached"}, sources(statuses(jsonBody(map[string]any{"ids": ids, "text": true}))))
	s.Equal(1, s.web.hitCount(s.genericsURL()), "a cache hit does not fetch")

	s.Equal([]string{"crawled"}, sources(statuses(jsonBody(map[string]any{"ids": ids[:1], "text": true, "maxAgeHours": 0}))), "maxAgeHours 0 always fetches")
	s.Equal(2, s.web.hitCount(s.genericsURL()))

	s.Require().True(s.cache.age(s.genericsURL(), 3*time.Hour))
	s.Equal([]string{"cached"}, sources(statuses(jsonBody(map[string]any{"ids": ids[:1], "text": true, "maxAgeHours": 4}))))
	s.Equal([]string{"crawled"}, sources(statuses(jsonBody(map[string]any{"ids": ids[:1], "text": true, "maxAgeHours": 2}))), "older than maxAgeHours is refetched")
	s.Equal(3, s.web.hitCount(s.genericsURL()))

	uncached := statuses(jsonBody(map[string]any{"urls": []string{s.web.newsURL("/about")}, "maxAgeHours": -1}))
	s.Require().Len(uncached, 1)
	s.Equal(websearch.ContentStatusError, uncached[0].Status, "maxAgeHours -1 is cache only")
	s.Equal(websearch.TagSourceNotAvailable, uncached[0].Error.Tag)
	s.Zero(s.web.hitCount(s.web.newsURL("/about")))
}

func (s *WebSearchIntegrationSuite) TestContentsLivecrawlWithMaxAgeHours() {
	body := jsonBody(map[string]any{"urls": []string{s.genericsURL()}, "livecrawl": "always", "maxAgeHours": 0})
	// The spec cannot express that the two fields exclude each other, so
	// it accepts this body; Exa's API rejects it, and so does Helix.
	resp := s.call(s.api, "/contents", body)
	errBody := s.requireError(resp, http.StatusBadRequest, websearch.TagInvalidRequest)
	s.Contains(errBody.Error, "maxAgeHours")
	s.Zero(s.web.hitCount(s.genericsURL()))
}

// --- /findSimilar ---

func (s *WebSearchIntegrationSuite) TestFindSimilar() {
	s.Run("excludeSourceDomain", func() {
		resp := s.call(s.api, "/findSimilar", jsonBody(map[string]any{
			"url": s.genericsURL(), "numResults": 3, "excludeSourceDomain": true,
			"contents": map[string]any{"highlights": true},
		}))
		s.requireOK(resp)
		body := decodeAs[websearch.FindSimilarResponse](s, resp)
		s.Equal([]string{s.rustURL(), s.web.blogURL("/posts/go-concurrency")}, resultURLs(body.Results),
			"other domains only, the page sharing more words with the description first")
		prev := 2.0
		for _, r := range body.Results {
			s.Require().NotNil(r.Score, r.URL)
			s.GreaterOrEqual(*r.Score, 0.0)
			s.LessOrEqual(*r.Score, prev, "sorted by score")
			prev = *r.Score
			s.NotEmpty(r.Highlights)
			s.Len(r.HighlightScores, len(r.Highlights), "findSimilar returns highlightScores")
		}
		s.Greater(*body.Results[0].Score, *body.Results[1].Score)
		s.Contains(s.firstPageQueries(), wsDescribeReply, "the LLM's description of the page is the search query")
	})
	s.Run("same domain allowed", func() {
		resp := s.call(s.api, "/findSimilar", jsonBody(map[string]any{"url": s.genericsURL()}))
		s.requireOK(resp)
		body := decodeAs[websearch.FindSimilarResponse](s, resp)
		urls := resultURLs(body.Results)
		s.NotContains(urls, s.genericsURL(), "the source page itself is never similar to itself")
		s.Contains(urls, s.web.newsURL("/articles/go-errors"))
	})
}

// --- /answer ---

func (s *WebSearchIntegrationSuite) TestAnswer() {
	s.Run("plain", func() {
		resp := s.call(s.api, "/answer", `{"query":"When did Go get generics?"}`)
		s.requireOK(resp)
		body := decodeAs[websearch.AnswerResponse](s, resp)
		s.Equal(wsAnswerReply, body.Answer)
		s.Require().Len(body.Citations, 4)
		s.Equal(s.genericsURL(), body.Citations[0].URL)
		for _, c := range body.Citations {
			s.Equal(c.URL, c.ID)
			s.Nil(c.Text, "text only when asked for")
		}
	})
	s.Run("text", func() {
		resp := s.call(s.api, "/answer", `{"query":"When did Go get generics?","text":true,"model":"exa-pro"}`)
		s.requireOK(resp)
		body := decodeAs[websearch.AnswerResponse](s, resp)
		s.Require().NotEmpty(body.Citations)
		s.Require().NotNil(body.Citations[0].Text)
		s.Contains(*body.Citations[0].Text, "type parameters")
		calls := s.llm.recorded()
		s.Equal(wsStrongModel, calls[len(calls)-1].Model, "exa-pro answers on the strong model")
	})
	s.Run("outputSchema", func() {
		resp := s.call(s.api, "/answer", jsonBody(map[string]any{
			"query": "When did Go get generics?",
			"outputSchema": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"version": map[string]any{"type": "string"}, "year": map[string]any{"type": "integer"}},
				"required":             []string{"version", "year"},
				"additionalProperties": false,
			},
		}))
		s.requireOK(resp)
		body := decodeAs[websearch.AnswerResponse](s, resp)
		s.Equal(map[string]any{"version": "Go 1.18 (version)", "year": float64(2026)}, body.Answer)
		calls := s.llm.recorded()
		last := calls[len(calls)-1]
		s.Equal("answer", last.SchemaName)
		s.True(last.Strict, "a closed schema uses strict structured output")
	})
}

func (s *WebSearchIntegrationSuite) TestAnswerStream() {
	resp := s.call(s.api, "/answer", `{"query":"When did Go get generics?","stream":true}`)
	s.requireOK(resp)
	s.Equal("text/event-stream", resp.header.Get("Content-Type"))
	events := resp.events

	var text strings.Builder
	var citations []websearch.Result
	var roles []string
	for _, raw := range events {
		var ev websearch.AnswerEvent
		s.Require().NoError(json.Unmarshal(raw, &ev))
		for _, c := range ev.Choices {
			text.WriteString(c.Delta.Content)
			if c.Delta.Role != "" {
				roles = append(roles, c.Delta.Role)
			}
		}
		if ev.Citations != nil {
			citations = ev.Citations
		}
	}
	s.Equal(wsAnswerReply, text.String())
	s.Equal([]string{"assistant"}, roles, "the role is sent once, on the first delta")
	s.Len(citations, 4)
}

// TestAnswerStreamFailure: an LLM failure after streaming started ends the
// stream with Exa's {"tag":"ERROR","payload":{...}} event, then [DONE].
func (s *WebSearchIntegrationSuite) TestAnswerStreamFailure() {
	resp := s.call(s.api, "/answer", `{"query":"When did Go get generics? interrupted","stream":true}`)
	s.requireOK(resp)
	s.Require().NotEmpty(resp.events)
	var last websearch.AnswerErrorEvent
	s.Require().NoError(json.Unmarshal(resp.events[len(resp.events)-1], &last))
	s.Equal(websearch.AnswerErrorTag, last.Tag)
	s.Equal(http.StatusInternalServerError, last.Payload.Error.Code)
	s.Contains(last.Payload.Error.Message, "upstream model overloaded")
}

// --- warnings ---

// TestWarningsOnContentsAndFindSimilar: ignored fields are reported on
// /contents and /findSimilar too.
func (s *WebSearchIntegrationSuite) TestWarningsOnContentsAndFindSimilar() {
	contents := s.call(s.api, "/contents", jsonBody(map[string]any{
		"urls": []string{s.genericsURL()}, "highlights": map[string]any{"highlightsPerUrl": 3},
	}))
	s.requireOK(contents)
	cw := decodeAs[websearch.ContentsResponse](s, contents).Warnings
	s.Require().Len(cw, 1)
	s.Equal(websearch.Warning{Code: websearch.WarningIgnoredField, Path: "/highlights/highlightsPerUrl", Message: "highlights.highlightsPerUrl is deprecated and ignored"}, cw[0])

	similar := s.call(s.api, "/findSimilar", jsonBody(map[string]any{"url": s.genericsURL(), "startCrawlDate": "2026-01-01T00:00:00Z"}))
	s.requireOK(similar)
	sw := decodeAs[websearch.FindSimilarResponse](s, similar).Warnings
	s.Require().Len(sw, 1)
	s.Equal(websearch.WarningIgnoredField, sw[0].Code)
	s.Equal("/startCrawlDate", sw[0].Path)
}

// --- errors ---

func (s *WebSearchIntegrationSuite) TestErrorEnvelopes() {
	s.Run("unknown field", func() {
		body := `{"query":"go generics","numResultz":3}`
		// Exa's SearchRequest is open, so the spec accepts this body; Helix
		// rejects unknown fields rather than silently ignore them (§7.1).
		resp := s.call(s.api, "/search", body)
		errBody := s.requireError(resp, http.StatusBadRequest, websearch.TagInvalidRequest)
		s.Contains(errBody.Error, "numResultz")
	})
	s.Run("numResults out of range", func() {
		body := `{"query":"go generics","numResults":500}`
		s.Error(s.spec.validateRequest("/search", []byte(body)), "the spec rejects it too")
		resp := s.send(s.api, "/search", body)
		errBody := s.requireError(resp, http.StatusBadRequest, websearch.TagInvalidRequest)
		s.Contains(errBody.Error, "numResults")
	})
	s.Run("compliance", func() {
		resp := s.call(s.api, "/search", `{"query":"go generics","compliance":"hipaa"}`)
		errBody := s.requireError(resp, http.StatusBadRequest, websearch.TagFeatureDisabled)
		s.Contains(errBody.Error, "compliance")
	})
	s.Run("contents with neither urls nor ids", func() {
		body := `{"text":true}`
		s.Error(s.spec.validateRequest("/contents", []byte(body)), "the spec rejects it too")
		resp := s.send(s.api, "/contents", body)
		s.requireError(resp, http.StatusBadRequest, websearch.TagInvalidRequest)
	})
	s.Empty(s.searx.requests(), "rejected requests never reach the search engine")
}

func (s *WebSearchIntegrationSuite) TestFeaturesNeedingAnLLMWithoutOne() {
	api := s.startAPI(wsStack{policy: netguard.Policy{AllowPrivate: true}, cache: newWSPageCache()})

	s.requireOK(s.call(api, "/search", `{"query":"go generics","contents":{"text":true}}`))

	for _, tc := range []struct {
		name, path, body, field string
	}{
		{"summary", "/search", `{"query":"go generics","contents":{"summary":{}}}`, "contents.summary"},
		{"highlights", "/contents", jsonBody(map[string]any{"urls": []string{s.genericsURL()}, "highlights": true}), "highlights"},
		{"deep search", "/search", `{"query":"go generics","type":"deep"}`, "type"},
		{"outputSchema", "/search", `{"query":"go generics","outputSchema":{"type":"text"}}`, "outputSchema"},
		{"streamed outputSchema", "/search", `{"query":"go generics","stream":true,"outputSchema":{"type":"text"}}`, "outputSchema"},
		{"answer", "/answer", `{"query":"When did Go get generics?"}`, "answer"},
		{"streamed answer", "/answer", `{"query":"When did Go get generics?","stream":true}`, "answer"},
		{"findSimilar", "/findSimilar", jsonBody(map[string]any{"url": s.genericsURL()}), "findSimilar"},
	} {
		s.Run(tc.name, func() {
			resp := s.call(api, tc.path, tc.body)
			errBody := s.requireError(resp, http.StatusBadRequest, websearch.TagFeatureDisabled)
			s.True(strings.HasPrefix(errBody.Error, tc.field+" is not supported"), errBody.Error)
		})
	}
	s.Empty(s.llm.recorded())
}
