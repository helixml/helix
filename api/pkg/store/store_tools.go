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

func (s *PostgresStore) CreateTool(ctx context.Context, tool *types.Tool) (*types.Tool, error) {
	if tool.ID == "" {
		tool.ID = system.GenerateToolID()
	}

	if tool.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	tool.Created = time.Now()

	err := s.gdb.WithContext(ctx).Create(&tool).Error
	if err != nil {
		return nil, err
	}
	return s.GetTool(ctx, tool.ID)
}

func (s *PostgresStore) UpdateTool(ctx context.Context, tool *types.Tool) (*types.Tool, error) {
	if tool.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if tool.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	tool.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Save(&tool).Error
	if err != nil {
		return nil, err
	}
	return s.GetTool(ctx, tool.ID)
}

func (s *PostgresStore) GetTool(ctx context.Context, id string) (*types.Tool, error) {
	var tool types.Tool
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&tool).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &tool, nil
}

func (s *PostgresStore) ListTools(ctx context.Context, q *ListToolsQuery) ([]*types.Tool, error) {
	var tools []*types.Tool
	err := s.gdb.WithContext(ctx).Where(&types.Tool{
		Owner:     q.Owner,
		OwnerType: q.OwnerType,
	}).Find(&tools).Error
	if err != nil {
		return nil, err
	}

	return tools, nil
}

func (s *PostgresStore) DeleteTool(ctx context.Context, id string) error {
	err := s.gdb.WithContext(ctx).Delete(&types.Tool{
		ID: id,
	}).Error
	if err != nil {
		return err
	}

	return nil
}
