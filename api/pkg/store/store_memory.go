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

// UpdateMemory updates the contents of a memory. The row is matched on ID,
// UserID and AppID so a caller can only modify memories they own for that app.
// Returns ErrNotFound if no matching row exists.
func (s *PostgresStore) UpdateMemory(ctx context.Context, memory *types.Memory) (*types.Memory, error) {
	if memory.ID == "" {
		return nil, fmt.Errorf("memory ID cannot be empty")
	}
	if memory.UserID == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}
	if memory.AppID == "" {
		return nil, fmt.Errorf("app ID cannot be empty")
	}

	memory.Updated = time.Now()
	res := s.gdb.WithContext(ctx).
		Model(&types.Memory{}).
		Where("id = ? AND user_id = ? AND app_id = ?", memory.ID, memory.UserID, memory.AppID).
		Updates(map[string]interface{}{
			"contents": memory.Contents,
			"updated":  memory.Updated,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}

	return memory, nil
}

// DeleteMemory deletes a memory. The row is matched on ID, UserID and AppID so
// a caller can only delete memories they own for that app. Returns ErrNotFound
// if no matching row exists.
func (s *PostgresStore) DeleteMemory(ctx context.Context, memory *types.Memory) error {
	if memory.ID == "" {
		return fmt.Errorf("memory ID cannot be empty")
	}
	if memory.UserID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}
	if memory.AppID == "" {
		return fmt.Errorf("app ID cannot be empty")
	}

	res := s.gdb.WithContext(ctx).
		Where("id = ? AND user_id = ? AND app_id = ?", memory.ID, memory.UserID, memory.AppID).
		Delete(&types.Memory{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
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

	err := s.gdb.WithContext(ctx).
		Where("user_id = ? AND app_id = ?", q.UserID, q.AppID).
		Order("created DESC").Find(&memories).Error
	if err != nil {
		return nil, err
	}
	return memories, nil
}
