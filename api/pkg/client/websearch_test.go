package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/websearch"
)

func newStreamServer(t *testing.T, body string) *HelixClient {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/websearch/answer", r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		require.JSONEq(t, `{"query":"q","stream":true}`, string(raw))
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, "key", false)
	require.NoError(t, err)
	return c
}

func TestStreamWebSearchAnswer(t *testing.T) {
	c := newStreamServer(t, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"4\"},\"finish_reason\":null}]}\n\n"+
		"data: {\"citations\":[{\"url\":\"https://a\",\"title\":\"A\"}]}\n\n"+
		"data: [DONE]\n\n")

	var text string
	var citations []websearch.Result
	err := c.StreamWebSearchAnswer(context.Background(), &websearch.AnswerRequest{Query: "q"}, func(ev websearch.AnswerEvent) error {
		for _, ch := range ev.Choices {
			text += ch.Delta.Content
		}
		citations = append(citations, ev.Citations...)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "4", text)
	require.Equal(t, "https://a", citations[0].URL)
}

func TestStreamWebSearchAnswerErrorEvent(t *testing.T) {
	c := newStreamServer(t, "data: {\"tag\":\"ERROR\",\"payload\":{\"error\":{\"code\":503,\"message\":\"down\"}}}\n\ndata: [DONE]\n\n")

	err := c.StreamWebSearchAnswer(context.Background(), &websearch.AnswerRequest{Query: "q"}, func(websearch.AnswerEvent) error { return nil })
	require.EqualError(t, err, "answer failed (503): down")
}

func TestStreamWebSearchAnswerTruncated(t *testing.T) {
	c := newStreamServer(t, "data: {\"choices\":[]}\n\n")

	err := c.StreamWebSearchAnswer(context.Background(), &websearch.AnswerRequest{Query: "q"}, func(websearch.AnswerEvent) error { return nil })
	require.ErrorContains(t, err, "without [DONE]")
}
