package rag

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/pgvector/pgvector-go"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

type PGVector struct {
	cfg             *config.ServerConfig
	providerManager manager.ProviderManager
	store           store.EmbeddingsStore
}

var _ RAG = &PGVector{}

func NewPGVector(cfg *config.ServerConfig, providerManager manager.ProviderManager, store store.EmbeddingsStore) *PGVector {
	return &PGVector{
		cfg:             cfg,
		providerManager: providerManager,
		store:           store,
	}
}

func (p *PGVector) Index(ctx context.Context, indexReqs ...*types.SessionRAGIndexChunk) error {
	embeddings, err := p.getEmbeddings(ctx, indexReqs)
	if err != nil {
		return err
	}

	return p.store.CreateKnowledgeEmbedding(ctx, embeddings...)
}

func (p *PGVector) getEmbeddings(ctx context.Context, indexReqs []*types.SessionRAGIndexChunk) ([]*types.KnowledgeEmbeddingItem, error) {
	var embeddings []*types.KnowledgeEmbeddingItem

	client, err := p.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: p.cfg.Embeddings.Provider,
	})
	if err != nil {
		return nil, err
	}

	for _, indexReq := range indexReqs {
		generated, err := client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
			Model: openai.EmbeddingModel(p.cfg.RAG.PGVector.EmbeddingsModel),
			Input: indexReq.Content,
		})
		if err != nil {
			return nil, err
		}

		if len(generated.Data) == 0 {
			log.Error().
				Str("knowledge_id", indexReq.DataEntityID).
				Msg("no embeddings returned for indexReq")
			continue
		}

		vector := pgvector.NewVector(generated.Data[0].Embedding)

		embedding := &types.KnowledgeEmbeddingItem{
			KnowledgeID:     indexReq.DataEntityID,
			DocumentID:      indexReq.DocumentID,
			DocumentGroupID: indexReq.DocumentGroupID,
			Content:         indexReq.Content,
			Embedding384:    vector,
			Source:          indexReq.Source,
		}

		dimensions, err := getDimensions(p.cfg.RAG.PGVector.EmbeddingsModel)
		if err != nil {
			return nil, err
		}

		switch dimensions {
		case Dimensions384:
			embedding.Embedding384 = vector
		case Dimensions1024:

		}

		embeddings = append(embeddings, embedding)
	}

	return embeddings, nil
}

func (p *PGVector) Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error) {

	client, err := p.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: p.cfg.Embeddings.Provider,
	})
	if err != nil {
		return nil, err
	}

	// Get the embeddings for the query
	generated, err := client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(p.cfg.RAG.PGVector.EmbeddingsModel),
		Input: q.Prompt,
	})
	if err != nil {
		return nil, err
	}

	query := &types.KnowledgeEmbeddingQuery{
		KnowledgeID: q.DataEntityID,
	}

	dimensions, err := getDimensions(p.cfg.RAG.PGVector.EmbeddingsModel)
	if err != nil {
		return nil, err
	}

	switch dimensions {
	case Dimensions384:
		query.Embedding384 = pgvector.NewVector(generated.Data[0].Embedding)
	}

	embeddings, err := p.store.QueryKnowledgeEmbeddings(ctx, query)
	if err != nil {
		return nil, err
	}

	var results []*types.SessionRAGResult

	for _, embedding := range embeddings {
		results = append(results, &types.SessionRAGResult{
			DocumentGroupID: embedding.DocumentGroupID,
			DocumentID:      embedding.DocumentID,
			Source:          embedding.Source,
			Content:         embedding.Content,
			ContentOffset:   embedding.ContentOffset,
		})
	}

	return results, nil
}

func (p *PGVector) Delete(ctx context.Context, req *types.DeleteIndexRequest) error {
	return p.store.DeleteKnowledgeEmbedding(ctx, req.DataEntityID)
}

type dimensions int

const (
	Dimensions384  dimensions = 384
	Dimensions1024 dimensions = 1024
)

// getDimensions - returns the dimensions of the embeddings for the given model
// Ref: https://huggingface.co/thenlper/gte-small
func getDimensions(model string) (dimensions, error) {
	switch model {
	case "thenlper/gte-small", "sentence-transformers/all-MiniLM-L6-v2", "sentence-transformers/all-MiniLM-L12-v2":
		return Dimensions384, nil
		// TODO: different dimensions for different models
	}

	return 0, fmt.Errorf("unknown model: %s", model)
}
