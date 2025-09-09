package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *PostgresStore) CreateMemory(ctx context.Context, memory *types.Memory) (*types.Memory, error) {
	if memory.ID == "" {
		memory.ID = system.GenerateMemoryID()
	}

	memory.Created = time.Now()
	memory.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Create(&memory).Error
	if err != nil {
		return nil, err
	}
	return memory, nil
}

func (s *PostgresStore) UpdateMemory(ctx context.Context, memory *types.Memory) (*types.Memory, error) {
	if memory.ID == "" {
		return nil, fmt.Errorf("failed to update memory")
	}

	memory.Updated = time.Now()
	err := s.gdb.WithContext(ctx).Save(&memory).Error
	if err != nil {
		return nil, err
	}

	return memory, nil
}

func (s *PostgresStore) DeleteMemory(ctx context.Context, memory *types.Memory) error {
	if memory.ID == "" {
		return fmt.Errorf("memory ID cannot be empty")
	}

	err := s.gdb.WithContext(ctx).Delete(&memory).Error
	if err != nil {
		return err
	}

	return nil
}

func (s *PostgresStore) ListMemories(ctx context.Context, q *types.ListMemoryRequest) ([]*types.Memory, error) {
	if q.AppID == "" {
		return nil, fmt.Errorf("app ID cannot be empty")
	}
	if q.UserID == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}

	var memories []*types.Memory

	query := s.gdb.WithContext(ctx)

	err := query.Order("created DESC").Find(&memories).Error
	if err != nil {
		return nil, err
	}
	return memories, nil
}
