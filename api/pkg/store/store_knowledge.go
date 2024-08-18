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
		knowledge.ID = system.GenerateKnowledgeID()
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

type LookupKnowledgeQuery struct {
	AppID string `json:"app_id"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

func (s *PostgresStore) LookupKnowledge(ctx context.Context, q *LookupKnowledgeQuery) (*types.Knowledge, error) {
	var knowledge types.Knowledge
	err := s.gdb.WithContext(ctx).Where(&types.Knowledge{
		AppID: q.AppID,
		ID:    q.ID,
		Name:  q.Name,
		Owner: q.Owner,
	}).First(&knowledge).Error
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
	State     types.KnowledgeState
	AppID     string
}

func (s *PostgresStore) ListKnowledge(ctx context.Context, q *ListKnowledgeQuery) ([]*types.Knowledge, error) {
	query := s.gdb.WithContext(ctx)

	if q.Owner != "" {
		query = query.Where("owner = ?", q.Owner)
	}
	if q.OwnerType != "" {
		query = query.Where("owner_type = ?", q.OwnerType)
	}
	if q.State != "" {
		query = query.Where("state = ?", q.State)
	}

	if q.AppID != "" {
		query = query.Where("app_id = ?", q.AppID)
	}

	var knowledgeList []*types.Knowledge

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
