package store

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

func (s *PostgresStore) CreateKnowledgeEmbedding(ctx context.Context, embeddings ...*types.KnowledgeEmbeddingItem) error {
	if len(embeddings) == 0 { // No embeddings to create
		return nil
	}

	// Ensure all embeddings have non-empty DocumentGroupID and DocumentID
	for _, embedding := range embeddings {
		if embedding.Owner == "" {
			return fmt.Errorf("owner is required")
		}

		if embedding.OwnerType == "" {
			return fmt.Errorf("owner type is required")
		}

		if embedding.KnowledgeID == "" {
			return fmt.Errorf("knowledge ID is required")
		}
	}

	err := s.gdb.WithContext(ctx).Create(embeddings).Error
	if err != nil {
		return err
	}
	return nil
}

func (s *PostgresStore) DeleteKnowledgeEmbedding(ctx context.Context, knowledgeID string) error {
	if knowledgeID == "" {
		return fmt.Errorf("knowledge ID is required")
	}

	err := s.gdb.WithContext(ctx).Where("knowledge_id = ?", knowledgeID).Delete(&types.KnowledgeEmbeddingItem{}).Error
	if err != nil {
		return err
	}
	return nil
}
