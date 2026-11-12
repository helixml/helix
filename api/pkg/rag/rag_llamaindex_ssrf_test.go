package rag

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/types"
)

// Custom RAG index/query/delete URLs are user input: the API server must not
// post to or read from internal or metadata addresses on their behalf.
func TestLlamaindexRejectsInternalDestinations(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer internal.Close()

	l := NewLlamaindex(&types.RAGSettings{
		IndexURL:  internal.URL + "/index",
		QueryURL:  "http://169.254.169.254/latest/meta-data/",
		DeleteURL: internal.URL + "/delete",
	}, netguard.Policy{})

	require.Error(t, l.Index(context.Background(), &types.SessionRAGIndexChunk{DataEntityID: "d", Content: "c"}))
	_, err := l.Query(context.Background(), &types.SessionRAGQuery{Prompt: "p", DataEntityID: "d"})
	require.Error(t, err)
	require.Error(t, l.Delete(context.Background(), &types.DeleteIndexRequest{DataEntityID: "d"}))
	require.Zero(t, hits.Load(), "internal listener was contacted")
}
