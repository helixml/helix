package store

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm/clause"
)

func (s *PGVectorStore) CreateKnowledgeEmbedding(ctx context.Context, embeddings ...*types.KnowledgeEmbeddingItem) error {
	if len(embeddings) == 0 { // No embeddings to create
		return nil
	}

	// Ensure all embeddings have non-empty DocumentGroupID and DocumentID
	for _, embedding := range embeddings {
		if embedding.DocumentGroupID == "" {
			return fmt.Errorf("document group ID is required")
		}

		if embedding.DocumentID == "" {
			return fmt.Errorf("document ID is required")
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

func (s *PGVectorStore) DeleteKnowledgeEmbedding(ctx context.Context, knowledgeID string) error {
	if knowledgeID == "" {
		return fmt.Errorf("knowledge ID is required")
	}

	err := s.gdb.WithContext(ctx).Where("knowledge_id = ?", knowledgeID).Delete(&types.KnowledgeEmbeddingItem{}).Error
	if err != nil {
		return err
	}
	return nil
}

func (s *PGVectorStore) QueryKnowledgeEmbeddings(ctx context.Context, q *types.KnowledgeEmbeddingQuery) ([]*types.KnowledgeEmbeddingItem, error) {
	if q.KnowledgeID == "" {
		return nil, fmt.Errorf("knowledge ID is required")
	}

	if len(q.Embedding384.Slice()) == 0 {
		return nil, fmt.Errorf("embedding is required")
	}

	if q.Limit == 0 {
		q.Limit = 5
	}

	var items []*types.KnowledgeEmbeddingItem
	err := s.gdb.WithContext(ctx).Where("knowledge_id = ?", q.KnowledgeID).Clauses(clause.OrderBy{
		Expression: clause.Expr{SQL: "embedding <-> ?", Vars: []interface{}{q.Embedding384}},
	}).Limit(q.Limit).Find(&items).Error
	if err != nil {
		return nil, err
	}

	return items, nil
}
