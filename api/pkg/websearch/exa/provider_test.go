package exa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/websearch"
)

type captured struct {
	method, path, apiKey, contentType string
	body                              map[string]any
}

// newServer serves one canned response per request and records what it got.
func newServer(t *testing.T, status int, contentType, body string) (*Provider, *captured) {
	t.Helper()
	c := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.method, c.path = r.Method, r.URL.Path
		c.apiKey, c.contentType = r.Header.Get("x-api-key"), r.Header.Get("Content-Type")
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(b, &c.body))
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	p, err := New("test-key", srv.URL+"/")
	require.NoError(t, err)
	return p, c
}

func TestNewRequiresAPIKey(t *testing.T) {
	_, err := New("", "https://api.exa.ai")
	require.Error(t, err)
}

func TestSearchRequestAndResponse(t *testing.T) {
	p, c := newServer(t, 200, "application/json", `{
		"requestId": "req_1",
		"resolvedSearchType": "neural",
		"searchTime": 123.4,
		"results": [{
			"id": "https://example.com/a", "url": "https://example.com/a", "title": "A",
			"publishedDate": "2025-01-02T00:00:00.000Z", "author": "Ann",
			"text": "body", "highlights": ["hi"], "summary": "sum",
			"extras": {"links": ["https://x"]}
		}],
		"costDollars": {"total": 0.005, "search": {"neural": 0.005}}
	}`)

	resp, err := p.Search(context.Background(), &websearch.SearchRequest{
		Query:      "golang",
		Type:       websearch.SearchTypeAuto,
		NumResults: 3,
		Contents: &websearch.ContentsOptions{
			Text:       &websearch.TextOption{Enabled: true},
			Highlights: &websearch.HighlightsOption{Enabled: true, HighlightsOptions: websearch.HighlightsOptions{MaxCharacters: 200}},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, c.method)
	assert.Equal(t, "/search", c.path)
	assert.Equal(t, "test-key", c.apiKey)
	assert.Equal(t, "application/json", c.contentType)
	assert.Equal(t, map[string]any{
		"query":      "golang",
		"type":       "auto",
		"numResults": float64(3),
		"contents": map[string]any{
			"text":       true,
			"highlights": map[string]any{"maxCharacters": float64(200)},
		},
	}, c.body)

	assert.Equal(t, "req_1", resp.RequestID)
	assert.Equal(t, "neural", resp.ResolvedSearchType)
	assert.InDelta(t, 123.4, resp.SearchTime, 1e-9)
	require.Len(t, resp.Results, 1)
	r := resp.Results[0]
	assert.Equal(t, "https://example.com/a", r.URL)
	assert.Equal(t, "Ann", *r.Author)
	assert.Equal(t, "body", *r.Text)
	assert.Equal(t, "sum", *r.Summary)
	assert.Equal(t, []string{"hi"}, r.Highlights)
	assert.Equal(t, time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), r.PublishedDate.UTC())
	assert.Equal(t, []string{"https://x"}, r.Extras.Links)
	assert.InDelta(t, 0.005, resp.CostDollars.Total, 1e-9)
}

func TestContentsInlinesOptions(t *testing.T) {
	p, c := newServer(t, 200, "application/json", `{
		"requestId": "req_2",
		"results": [{"id": "https://example.com", "url": "https://example.com", "title": "E", "text": "t"}],
		"statuses": [{"id": "https://example.com", "status": "success", "source": "cached"},
		             {"id": "https://bad", "status": "error", "error": {"tag": "CRAWL_NOT_FOUND", "httpStatusCode": 404}}]
	}`)

	resp, err := p.Contents(context.Background(), &websearch.ContentsRequest{
		URLs: []string{"https://example.com", "https://bad"},
		ContentsOptions: websearch.ContentsOptions{
			Text:       &websearch.TextOption{Enabled: true, TextOptions: websearch.TextOptions{MaxCharacters: 1000}},
			Highlights: &websearch.HighlightsOption{Enabled: true},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "/contents", c.path)
	assert.Equal(t, map[string]any{
		"urls":       []any{"https://example.com", "https://bad"},
		"text":       map[string]any{"maxCharacters": float64(1000)},
		"highlights": true,
	}, c.body)
	require.Len(t, resp.Statuses, 2)
	assert.Equal(t, websearch.ContentSourceCached, resp.Statuses[0].Source)
	assert.Equal(t, websearch.TagCrawlNotFound, resp.Statuses[1].Error.Tag)
	assert.Equal(t, 404, resp.Statuses[1].Error.HTTPStatusCode)
}

func TestFindSimilar(t *testing.T) {
	p, c := newServer(t, 200, "application/json", `{"requestId":"r","results":[{"url":"https://b","title":"B","score":0.9}]}`)
	resp, err := p.FindSimilar(context.Background(), &websearch.FindSimilarRequest{URL: "https://a", ExcludeSourceDomain: true})
	require.NoError(t, err)
	assert.Equal(t, "/findSimilar", c.path)
	assert.Equal(t, map[string]any{"url": "https://a", "excludeSourceDomain": true}, c.body)
	assert.InDelta(t, 0.9, *resp.Results[0].Score, 1e-9)
}

func TestAnswer(t *testing.T) {
	p, c := newServer(t, 200, "application/json", `{"requestId":"r","answer":"42","citations":[{"id":"https://c","url":"https://c","title":"C"}],"costDollars":{"total":0.01}}`)
	resp, err := p.Answer(context.Background(), &websearch.AnswerRequest{Query: "meaning", Text: true})
	require.NoError(t, err)
	assert.Equal(t, "/answer", c.path)
	assert.Equal(t, map[string]any{"query": "meaning", "text": true}, c.body)
	assert.Equal(t, "42", resp.Answer)
	require.Len(t, resp.Citations, 1)
	assert.Equal(t, "https://c", resp.Citations[0].URL)
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantTag     string
		wantMsg     string
	}{
		{"400 with tag", 400, "application/json", `{"requestId":"r","error":"numResults must be <= 100","tag":"INVALID_REQUEST_BODY"}`, "INVALID_REQUEST_BODY", "numResults must be <= 100"},
		{"402 credits", 402, "application/json", `{"requestId":"r","error":"You have exceeded your credits limit","tag":"NO_MORE_CREDITS"}`, "NO_MORE_CREDITS", "You have exceeded your credits limit"},
		{"non-JSON 503", 503, "text/html", `<html>Service Unavailable</html>`, websearch.TagUpstreamError, "exa returned 503: <html>Service Unavailable</html>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := newServer(t, tc.status, tc.contentType, tc.body)
			_, err := p.Search(context.Background(), &websearch.SearchRequest{Query: "q"})
			var we *websearch.Error
			require.ErrorAs(t, err, &we)
			assert.Equal(t, tc.status, we.Status)
			assert.Equal(t, tc.wantTag, we.Tag)
			assert.Equal(t, tc.wantMsg, we.Message)
		})
	}
}

func TestTransportErrorIs502(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	p, err := New("k", url)
	require.NoError(t, err)

	_, err = p.Search(context.Background(), &websearch.SearchRequest{Query: "q"})
	var we *websearch.Error
	require.ErrorAs(t, err, &we)
	assert.Equal(t, http.StatusBadGateway, we.Status)
	assert.Equal(t, websearch.TagUpstreamError, we.Tag)
}

func TestStreamSearch(t *testing.T) {
	sse := "data: {\"type\":\"text-delta\",\"delta\":\"Hel\"}\n\n" +
		": keepalive\n\n" +
		"data: {\"type\":\"text-delta\",\"delta\":\"lo\"}\r\n\r\n" +
		"data: {\"type\":\"results\",\"results\":[{\"url\":\"https://a\",\"title\":\"A\"}]}\n\n" +
		"data: {\"type\":\"done\",\"searchTime\":5,\"costDollars\":{\"total\":0.02}}\n\n" +
		"data: [DONE]\n\n" +
		"data: {\"type\":\"text-delta\",\"delta\":\"after done\"}\n\n"
	p, c := newServer(t, 200, "text/event-stream", sse)

	var events []websearch.SearchEvent
	err := p.StreamSearch(context.Background(), &websearch.SearchRequest{Query: "q", OutputSchema: json.RawMessage(`{"type":"text"}`)},
		func(ev websearch.SearchEvent) error {
			events = append(events, ev)
			return nil
		})
	require.NoError(t, err)

	assert.Equal(t, "/search", c.path)
	assert.Equal(t, true, c.body["stream"])
	assert.Equal(t, map[string]any{"type": "text"}, c.body["outputSchema"])
	require.Len(t, events, 4)
	assert.Equal(t, "Hel", *events[0].Delta)
	assert.Equal(t, "lo", *events[1].Delta)
	assert.Equal(t, "https://a", events[2].Results[0].URL)
	assert.Equal(t, websearch.SearchEventDone, events[3].Type)
	assert.InDelta(t, 0.02, events[3].CostDollars.Total, 1e-9)
}

func TestStreamAnswer(t *testing.T) {
	sse := "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Fo\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"rty-two\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: {\"citations\":[{\"url\":\"https://c\",\"title\":\"C\"}]}\n\n" +
		"data: {\"costDollars\":{\"total\":0.005}}\n\n" +
		"data: [DONE]\n\n"
	p, c := newServer(t, 200, "text/event-stream", sse)

	var events []websearch.AnswerEvent
	err := p.StreamAnswer(context.Background(), &websearch.AnswerRequest{Query: "q"}, func(ev websearch.AnswerEvent) error {
		events = append(events, ev)
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, "/answer", c.path)
	assert.Equal(t, true, c.body["stream"])
	require.Len(t, events, 4)
	assert.Equal(t, "Fo", events[0].Choices[0].Delta.Content)
	assert.Nil(t, events[0].Choices[0].FinishReason)
	assert.Equal(t, "stop", *events[1].Choices[0].FinishReason)
	assert.Equal(t, "https://c", events[2].Citations[0].URL)
	assert.InDelta(t, 0.005, events[3].CostDollars.Total, 1e-9)
}

func TestStreamAnswerErrorEvent(t *testing.T) {
	sse := "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"x\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"tag\":\"ERROR\",\"payload\":{\"error\":{\"code\":429,\"message\":\"rate limited\"}}}\n\n" +
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"never\"},\"finish_reason\":null}]}\n\n"
	p, _ := newServer(t, 200, "text/event-stream", sse)

	var n int
	err := p.StreamAnswer(context.Background(), &websearch.AnswerRequest{Query: "q"}, func(websearch.AnswerEvent) error {
		n++
		return nil
	})
	var we *websearch.Error
	require.ErrorAs(t, err, &we)
	assert.Equal(t, 429, we.Status)
	assert.Equal(t, websearch.TagUpstreamError, we.Tag)
	assert.Equal(t, "rate limited", we.Message)
	assert.Equal(t, 1, n)
}

func TestStreamHTTPErrorBeforeEvents(t *testing.T) {
	p, _ := newServer(t, 401, "application/json", `{"requestId":"r","error":"Invalid API key","tag":"INVALID_API_KEY"}`)
	err := p.StreamAnswer(context.Background(), &websearch.AnswerRequest{Query: "q"}, func(websearch.AnswerEvent) error {
		t.Fatal("emit must not be called")
		return nil
	})
	var we *websearch.Error
	require.ErrorAs(t, err, &we)
	assert.Equal(t, 401, we.Status)
	assert.Equal(t, "INVALID_API_KEY", we.Tag)
}

func TestStreamStopsOnEmitError(t *testing.T) {
	p, _ := newServer(t, 200, "text/event-stream",
		"data: {\"type\":\"text-delta\",\"delta\":\"a\"}\n\ndata: {\"type\":\"text-delta\",\"delta\":\"b\"}\n\n")
	stop := errors.New("client gone")
	var n int
	err := p.StreamSearch(context.Background(), &websearch.SearchRequest{Query: "q"}, func(websearch.SearchEvent) error {
		n++
		return stop
	})
	require.ErrorIs(t, err, stop)
	assert.Equal(t, 1, n)
}

func TestStreamContextCancel(t *testing.T) {
	released := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"text-delta\",\"delta\":\"a\"}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-released:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(released) })
	p, err := New("k", srv.URL)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	err = p.StreamSearch(ctx, &websearch.SearchRequest{Query: "q"}, func(websearch.SearchEvent) error {
		cancel()
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestSearchContextCancel(t *testing.T) {
	released := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-released:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(released) })
	p, err := New("k", srv.URL)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = p.Search(ctx, &websearch.SearchRequest{Query: "q"})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

// TestLiveExa runs against the real Exa API when EXA_API_KEY is set.
func TestLiveExa(t *testing.T) {
	key := os.Getenv("EXA_API_KEY")
	if key == "" {
		t.Skip("EXA_API_KEY not set")
	}
	p, err := New(key, "https://api.exa.ai")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sr, err := p.Search(ctx, &websearch.SearchRequest{
		Query:      "Go programming language release notes",
		NumResults: 2,
		Contents:   &websearch.ContentsOptions{Highlights: &websearch.HighlightsOption{Enabled: true}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, sr.Results)
	assert.NotEmpty(t, sr.RequestID)

	cr, err := p.Contents(ctx, &websearch.ContentsRequest{
		URLs:            []string{sr.Results[0].URL},
		ContentsOptions: websearch.ContentsOptions{Text: &websearch.TextOption{Enabled: true, TextOptions: websearch.TextOptions{MaxCharacters: 500}}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, cr.Statuses)

	ar, err := p.Answer(ctx, &websearch.AnswerRequest{Query: "What is the latest stable Go version?"})
	require.NoError(t, err)
	assert.NotNil(t, ar.Answer)

}
