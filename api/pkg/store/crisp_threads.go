package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateCrispThread(ctx context.Context, thread *types.CrispThread) (*types.CrispThread, error) {
	if thread.CrispSessionID == "" {
		return nil, ErrNotFound
	}

	if thread.AppID == "" {
		return nil, ErrNotFound
	}

	if thread.Created.IsZero() {
		thread.Created = time.Now()
	}

	if thread.Updated.IsZero() {
		thread.Updated = time.Now()
	}

	err := s.gdb.WithContext(ctx).Create(thread).Error
	if err != nil {
		return nil, err
	}

	return thread, nil
}

func (s *PostgresStore) GetCrispThread(ctx context.Context, appID, crispSessionID string) (*types.CrispThread, error) {
	if appID == "" || crispSessionID == "" {
		return nil, ErrNotFound
	}

	var thread types.CrispThread
	err := s.gdb.WithContext(ctx).
		Where("app_id = ? AND crisp_session_id = ?", appID, crispSessionID).
		First(&thread).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &thread, nil
}

func (s *PostgresStore) DeleteCrispThread(ctx context.Context, olderThan time.Time) error {
	if olderThan.IsZero() {
		return nil
	}

	err := s.gdb.WithContext(ctx).
		Where("updated < ?", olderThan).
		Delete(&types.CrispThread{}).Error

	if err != nil {
		return err
	}

	return nil
}
