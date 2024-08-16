package rag

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

//go:generate mockgen -source $GOFILE -destination rag_mocks.go -package $GOPACKAGE

type RAG interface {
	Index(ctx context.Context, req *types.SessionRAGIndexChunk) error
	Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error)
}
