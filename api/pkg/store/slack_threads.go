package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateSlackThread(ctx context.Context, thread *types.SlackThread) (*types.SlackThread, error) {
	if thread.ThreadKey == "" {
		return nil, ErrNotFound
	}

	if thread.AppID == "" {
		return nil, ErrNotFound
	}

	if thread.Channel == "" {
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

func (s *PostgresStore) GetSlackThread(ctx context.Context, appID, channel, threadKey string) (*types.SlackThread, error) {
	if appID == "" || channel == "" || threadKey == "" {
		return nil, ErrNotFound
	}

	var thread types.SlackThread
	err := s.gdb.WithContext(ctx).
		Where("app_id = ? AND channel = ? AND thread_key = ?", appID, channel, threadKey).
		First(&thread).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &thread, nil
}

func (s *PostgresStore) GetSlackThreadBySpecTaskID(ctx context.Context, appID, channel, specTaskID string) (*types.SlackThread, error) {
	if appID == "" || channel == "" || specTaskID == "" {
		return nil, ErrNotFound
	}

	var thread types.SlackThread
	err := s.gdb.WithContext(ctx).
		Where("app_id = ? AND channel = ? AND spec_task_id = ?", appID, channel, specTaskID).
		First(&thread).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &thread, nil
}

func (s *PostgresStore) DeleteSlackThread(ctx context.Context, olderThan time.Time) error {
	if olderThan.IsZero() {
		return nil
	}

	err := s.gdb.WithContext(ctx).
		Where("updated < ?", olderThan).
		Delete(&types.SlackThread{}).Error

	if err != nil {
		return err
	}

	return nil
}
