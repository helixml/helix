package rag

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

type RAG interface {
	Index(ctx context.Context, req *types.SessionRAGIndexChunk) error
	Query(ctx context.Context, q *types.SessionRAGQuery) (*types.SessionRAGResult, error)
}
