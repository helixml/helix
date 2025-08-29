package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *PostgresStore) CreateSlot(ctx context.Context, slot *types.RunnerSlot) (*types.RunnerSlot, error) {
	if slot.ID == uuid.Nil {
		return nil, errors.New("slot ID is required")
	}
	if slot.RunnerID == "" {
		return nil, errors.New("runner ID is required")
	}
	if slot.Model == "" {
		return nil, errors.New("model is required")
	}

	db := s.gdb.WithContext(ctx)

	// Use upsert to handle cases where slot already exists
	err := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(&slot).Error
	if err != nil {
		return nil, err
	}

	return slot, nil
}

func (s *PostgresStore) GetSlot(ctx context.Context, id string) (*types.RunnerSlot, error) {
	if id == "" {
		return nil, errors.New("slot ID is required")
	}

	db := s.gdb.WithContext(ctx)

	var slot types.RunnerSlot
	err := db.Where("id = ?", id).First(&slot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &slot, nil
}

func (s *PostgresStore) UpdateSlot(ctx context.Context, slot *types.RunnerSlot) (*types.RunnerSlot, error) {
	if slot.ID == uuid.Nil {
		return nil, errors.New("slot ID is required")
	}
	if slot.RunnerID == "" {
		return nil, errors.New("runner ID is required")
	}

	db := s.gdb.WithContext(ctx)

	slot.Updated = time.Now()

	err := db.Save(&slot).Error
	if err != nil {
		return nil, err
	}

	return slot, nil
}

func (s *PostgresStore) DeleteSlot(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("slot ID is required")
	}

	db := s.gdb.WithContext(ctx)

	err := db.Delete(&types.RunnerSlot{}, "id = ?", id).Error
	if err != nil {
		return err
	}

	return nil
}

func (s *PostgresStore) ListSlots(ctx context.Context, runnerID string) ([]*types.RunnerSlot, error) {
	if runnerID == "" {
		return nil, errors.New("runner ID is required")
	}

	db := s.gdb.WithContext(ctx)

	var slots []*types.RunnerSlot
	err := db.Where("runner_id = ?", runnerID).Order("created ASC").Find(&slots).Error
	if err != nil {
		return nil, err
	}

	return slots, nil
}

func (s *PostgresStore) ListAllSlots(ctx context.Context) ([]*types.RunnerSlot, error) {
	db := s.gdb.WithContext(ctx)

	var slots []*types.RunnerSlot
	err := db.Order("runner_id ASC, created ASC").Find(&slots).Error
	if err != nil {
		return nil, err
	}

	return slots, nil
}
