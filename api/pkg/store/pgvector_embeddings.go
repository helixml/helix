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

		if embedding.DataEntityID == "" {
			return fmt.Errorf("data entity ID is required")
		}

		err := validateEmbedding(embedding)
		if err != nil {
			return err
		}
	}

	err := s.gdb.WithContext(ctx).Create(embeddings).Error
	if err != nil {
		return err
	}
	return nil
}

func validateEmbedding(embedding *types.KnowledgeEmbeddingItem) error {
	if embedding.Embedding384 == nil && embedding.Embedding512 == nil && embedding.Embedding1024 == nil && embedding.Embedding1536 == nil && embedding.Embedding3584 == nil {
		return fmt.Errorf("no embedding provided")
	}

	return nil
}

func (s *PGVectorStore) DeleteKnowledgeEmbedding(ctx context.Context, dataEntityID string) error {
	if dataEntityID == "" {
		return fmt.Errorf("data entity ID is required")
	}

	err := s.gdb.WithContext(ctx).Where("data_entity_id = ?", dataEntityID).Unscoped().Delete(&types.KnowledgeEmbeddingItem{}).Error
	if err != nil {
		return err
	}
	return nil
}

func (s *PGVectorStore) QueryKnowledgeEmbeddings(ctx context.Context, q *types.KnowledgeEmbeddingQuery) ([]*types.KnowledgeEmbeddingItem, error) {
	if q.DataEntityID == "" {
		return nil, fmt.Errorf("data entity ID is required")
	}

	if q.Limit == 0 {
		q.Limit = 5
	}

	var items []*types.KnowledgeEmbeddingItem

	query := s.gdb.WithContext(ctx).Distinct().Where("data_entity_id = ?", q.DataEntityID)

	var conditions []string
	var vars []interface{}

	if q.Content != "" {
		conditions = append(conditions, "content @@ websearch_to_tsquery(?)")
		vars = append(vars, q.Content)
	}

	var orderBy string
	var orderVars []interface{}

	switch {
	case len(q.Embedding384.Slice()) > 0:
		orderBy = "embedding384 <-> ?"
		orderVars = []interface{}{q.Embedding384}
	case len(q.Embedding512.Slice()) > 0:
		orderBy = "embedding512 <-> ?"
		orderVars = []interface{}{q.Embedding512}
	case len(q.Embedding1024.Slice()) > 0:
		orderBy = "embedding1024 <-> ?"
		orderVars = []interface{}{q.Embedding1024}
	case len(q.Embedding1536.Slice()) > 0:
		orderBy = "embedding1536 <-> ?"
		orderVars = []interface{}{q.Embedding1536}
	case len(q.Embedding3584.Slice()) > 0:
		orderBy = "embedding3584 <-> ?"
		orderVars = []interface{}{q.Embedding3584}
	}

	if orderBy != "" {
		conditions = append(conditions, orderBy+" < 1.0")
		vars = append(vars, orderVars...)
	}

	if len(conditions) > 0 {
		query = query.Where("("+conditions[0]+")", vars[0])
		for i := 1; i < len(conditions); i++ {
			query = query.Or("("+conditions[i]+")", vars[i])
		}
	}

	if orderBy != "" {
		query = query.Order(clause.OrderBy{
			Expression: clause.Expr{SQL: orderBy, Vars: orderVars},
		})
	}

	err := query.Debug().Limit(q.Limit).Find(&items).Error
	if err != nil {
		return nil, err
	}

	return items, nil
}
