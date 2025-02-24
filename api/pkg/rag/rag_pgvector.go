package rag

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/pgvector/pgvector-go"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
	"github.com/sourcegraph/conc/pool"
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

	start := time.Now()
	err = p.store.CreateKnowledgeEmbedding(ctx, embeddings...)
	if err != nil {
		return err
	}

	log.Info().
		Int("duration_ms", int(time.Since(start).Milliseconds())).
		Int("embeddings", len(embeddings)).
		Msg("inserted embeddings into pgvector")

	return nil
}

func (p *PGVector) getEmbeddings(ctx context.Context, indexReqs []*types.SessionRAGIndexChunk) ([]*types.KnowledgeEmbeddingItem, error) {
	var embeddings []*types.KnowledgeEmbeddingItem

	client, err := p.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: p.cfg.RAG.PGVector.Provider,
	})
	if err != nil {
		return nil, err
	}

	pool := pool.New().
		WithMaxGoroutines(p.cfg.RAG.PGVector.EmbeddingsConcurrency).
		WithErrors()

	mu := sync.Mutex{}

	for _, indexReq := range indexReqs {
		indexReq := indexReq
		pool.Go(func() error {
			start := time.Now()
			generated, err := client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
				Model: openai.EmbeddingModel(p.cfg.RAG.PGVector.EmbeddingsModel),
				Input: indexReq.Content,
			})
			if err != nil {
				log.Error().
					Err(err).
					Str("model", p.cfg.RAG.PGVector.EmbeddingsModel).
					Int("content_length", len(indexReq.Content)).
					Str("knowledge_id", indexReq.DataEntityID).
					Msg("failed to create embeddings")
				return err
			}

			if len(generated.Data) == 0 {
				log.Error().
					Str("knowledge_id", indexReq.DataEntityID).
					Msg("no embeddings returned for indexReq")
				return nil
			}

			log.Info().
				Str("knowledge_id", indexReq.DataEntityID).
				Str("model", p.cfg.RAG.PGVector.EmbeddingsModel).
				Int("content_length", len(indexReq.Content)).
				Int("duration_ms", int(time.Since(start).Milliseconds())).
				Msg("created embeddings")

			vector := pgvector.NewVector(generated.Data[0].Embedding)

			embedding := &types.KnowledgeEmbeddingItem{
				DataEntityID:    indexReq.DataEntityID,
				DocumentID:      indexReq.DocumentID,
				DocumentGroupID: indexReq.DocumentGroupID,
				Content:         indexReq.Content,
				ContentOffset:   indexReq.ContentOffset,
				Source:          indexReq.Source,
				EmbeddingsModel: p.cfg.RAG.PGVector.EmbeddingsModel,
			}

			dimensions, err := p.getDimensions(p.cfg.RAG.PGVector.EmbeddingsModel)
			if err != nil {
				return err
			}

			switch dimensions {
			case types.Dimensions384:
				embedding.Embedding384 = &vector
			case types.Dimensions512:
				embedding.Embedding512 = &vector
			case types.Dimensions1024:
				embedding.Embedding1024 = &vector
			case types.Dimensions1536:
				embedding.Embedding1536 = &vector
			case types.Dimensions3584:
				embedding.Embedding3584 = &vector
			}

			mu.Lock()
			embeddings = append(embeddings, embedding)
			mu.Unlock()

			return nil
		})
	}

	err = pool.Wait()
	if err != nil {
		return nil, err
	}

	return embeddings, nil
}

func (p *PGVector) Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error) {

	client, err := p.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: p.cfg.RAG.PGVector.Provider,
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
		DataEntityID: q.DataEntityID,
		Limit:        q.MaxResults,
		Content:      q.Prompt,
	}

	dimensions, err := p.getDimensions(p.cfg.RAG.PGVector.EmbeddingsModel)
	if err != nil {
		return nil, err
	}

	switch dimensions {
	case types.Dimensions384:
		query.Embedding384 = pgvector.NewVector(generated.Data[0].Embedding)
	case types.Dimensions512:
		query.Embedding512 = pgvector.NewVector(generated.Data[0].Embedding)
	case types.Dimensions1024:
		query.Embedding1024 = pgvector.NewVector(generated.Data[0].Embedding)
	case types.Dimensions1536:
		query.Embedding1536 = pgvector.NewVector(generated.Data[0].Embedding)
	case types.Dimensions3584:
		query.Embedding3584 = pgvector.NewVector(generated.Data[0].Embedding)
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

func (p *PGVector) getDimensions(model string) (types.Dimensions, error) {
	if p.cfg.RAG.PGVector.Dimensions != 0 {
		return p.cfg.RAG.PGVector.Dimensions, nil
	}

	return getDimensions(model)
}

// getDimensions - returns the dimensions of the embeddings for the given model
// Ref: https://huggingface.co/thenlper/gte-small
func getDimensions(model string) (types.Dimensions, error) {
	switch model {
	case "thenlper/gte-small", "sentence-transformers/all-MiniLM-L6-v2", "sentence-transformers/all-MiniLM-L12-v2":
		return types.Dimensions384, nil
	case "thenlper/gte-base":
		return types.Dimensions512, nil
	case "thenlper/gte-large":
		return types.Dimensions1024, nil
	case "text-embedding-3-small":
		return types.Dimensions1536, nil
	case "Alibaba-NLP/gte-Qwen2-7B-instruct":
		return types.Dimensions3584, nil
	}

	return 0, fmt.Errorf("unknown model: %s", model)
}
