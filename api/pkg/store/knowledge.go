package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateKnowledge(ctx context.Context, knowledge *types.Knowledge) (*types.Knowledge, error) {
	if knowledge.ID == "" {
		knowledge.ID = system.GenerateUUID()
	}

	if knowledge.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	knowledge.Created = time.Now()
	knowledge.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Create(knowledge).Error
	if err != nil {
		return nil, err
	}
	return s.GetKnowledge(ctx, knowledge.ID)
}

func (s *PostgresStore) GetKnowledge(ctx context.Context, id string) (*types.Knowledge, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}

	var knowledge types.Knowledge
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&knowledge).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &knowledge, nil
}

func (s *PostgresStore) UpdateKnowledge(ctx context.Context, knowledge *types.Knowledge) (*types.Knowledge, error) {
	if knowledge.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if knowledge.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	knowledge.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Save(knowledge).Error
	if err != nil {
		return nil, err
	}
	return s.GetKnowledge(ctx, knowledge.ID)
}

type ListKnowledgeQuery struct {
	Owner     string
	OwnerType types.OwnerType
	Type      types.DataEntityType
}

func (s *PostgresStore) ListKnowledge(ctx context.Context, q *ListKnowledgeQuery) ([]*types.Knowledge, error) {
	var knowledgeList []*types.Knowledge
	query := s.gdb.WithContext(ctx)

	if q.Owner != "" {
		query = query.Where("owner = ?", q.Owner)
	}
	if q.OwnerType != "" {
		query = query.Where("owner_type = ?", q.OwnerType)
	}
	if q.Type != "" {
		query = query.Where("type = ?", q.Type)
	}

	err := query.Find(&knowledgeList).Error
	if err != nil {
		return nil, err
	}

	return knowledgeList, nil
}

func (s *PostgresStore) DeleteKnowledge(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.Knowledge{ID: id}).Error
	if err != nil {
		return err
	}
	return nil
}
