package rag

import (
	"context"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
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
	return nil
}

func (p *PGVector) Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error) {
	return nil, nil
}

func (p *PGVector) Delete(ctx context.Context, req *types.DeleteIndexRequest) error {
	return nil
}
