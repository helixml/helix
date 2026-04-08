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

// PageImageRenderer is an optional interface implemented by RAG backends that
// can render document page images. When the inference path detects results
// with page metadata, it checks if the RAG client implements this interface
// and renders page images as base64 data URIs for multimodal LLM input.
type PageImageRenderer interface {
	RenderPageImage(ctx context.Context, dataEntityID string, filePath string, page int) ([]byte, error)
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
