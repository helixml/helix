package rag

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

//go:generate mockgen -source $GOFILE -destination rag_mocks.go -package $GOPACKAGE

type RAG interface {
	Index(ctx context.Context, req ...*types.SessionRAGIndexChunk) error
	Query(ctx context.Context, q *types.SessionRAGQuery) ([]*types.SessionRAGResult, error)
	Delete(ctx context.Context, req *types.DeleteIndexRequest) error
}

// KoditIndexer is an optional interface implemented by RAG backends that use Kodit
// to index a local directory. When a RAG client implements this interface, the
// knowledge reconciler calls RegisterDirectory instead of the normal extraction pipeline.
type KoditIndexer interface {
	// RegisterDirectory registers a local filestore directory with Kodit and stores
	// the resulting kodit repository ID on the corresponding DataEntity.
	// owner and ownerType are passed to create/update the DataEntity record.
	RegisterDirectory(ctx context.Context, dataEntityID, localPath, owner, ownerType string) error
}
