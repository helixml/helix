package rag

import (
	"context"

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
	store           store.Store
}

var _ RAG = &PGVector{}

func NewPGVector(cfg *config.ServerConfig, providerManager manager.ProviderManager, store store.Store) *PGVector {
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

		embeddings = append(embeddings, &types.KnowledgeEmbeddingItem{
			KnowledgeID:     indexReq.DataEntityID,
			DocumentID:      indexReq.DocumentID,
			DocumentGroupID: indexReq.DocumentGroupID,
			Content:         indexReq.Content,
			Embedding:       vector,
			Source:          indexReq.Source,
			Filename:        indexReq.Filename,
		})
	}

	return embeddings, nil
}

func (p *PGVector) Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error) {
	return nil, nil
}

func (p *PGVector) Delete(ctx context.Context, req *types.DeleteIndexRequest) error {
	return nil
}
