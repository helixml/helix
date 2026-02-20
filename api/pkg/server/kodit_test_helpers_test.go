package server

import (
	"context"

	"github.com/helixml/kodit/infrastructure/provider"
)

// noopEmbedder is a no-op embedding provider for tests that don't need real
// embeddings. It satisfies the provider.Embedder interface so kodit.New()
// can be called without a local model or external endpoint.
type noopEmbedder struct{}

func (noopEmbedder) Embed(_ context.Context, req provider.EmbeddingRequest) (provider.EmbeddingResponse, error) {
	dim := 384 // matches st-codesearch-distilroberta-base
	embeddings := make([][]float64, len(req.Texts()))
	for i := range embeddings {
		embeddings[i] = make([]float64, dim)
	}
	return provider.NewEmbeddingResponse(embeddings, provider.NewUsage(0, 0, 0)), nil
}
