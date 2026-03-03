package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateTelegramThread(ctx context.Context, thread *types.TelegramThread) (*types.TelegramThread, error) {
	if thread.TelegramChatID == 0 {
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

func (s *PostgresStore) GetTelegramThread(ctx context.Context, appID string, telegramChatID int64) (*types.TelegramThread, error) {
	if appID == "" || telegramChatID == 0 {
		return nil, ErrNotFound
	}

	var thread types.TelegramThread
	err := s.gdb.WithContext(ctx).
		Where("app_id = ? AND telegram_chat_id = ?", appID, telegramChatID).
		First(&thread).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &thread, nil
}

// GetTelegramThreadByChatID finds a thread by chat ID across all apps
func (s *PostgresStore) GetTelegramThreadByChatID(ctx context.Context, telegramChatID int64) (*types.TelegramThread, error) {
	if telegramChatID == 0 {
		return nil, ErrNotFound
	}

	var thread types.TelegramThread
	err := s.gdb.WithContext(ctx).
		Where("telegram_chat_id = ?", telegramChatID).
		Order("updated DESC").
		First(&thread).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &thread, nil
}

func (s *PostgresStore) UpdateTelegramThread(ctx context.Context, thread *types.TelegramThread) (*types.TelegramThread, error) {
	if thread.TelegramChatID == 0 || thread.AppID == "" {
		return nil, ErrNotFound
	}

	thread.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Save(thread).Error
	if err != nil {
		return nil, err
	}

	return thread, nil
}

// ListTelegramThreadsWithUpdates returns all threads that have updates enabled for a given project
func (s *PostgresStore) ListTelegramThreadsWithUpdates(ctx context.Context, projectID string) ([]*types.TelegramThread, error) {
	if projectID == "" {
		return nil, nil
	}

	var threads []*types.TelegramThread
	err := s.gdb.WithContext(ctx).
		Where("project_id = ? AND updates = ?", projectID, true).
		Find(&threads).Error

	if err != nil {
		return nil, err
	}

	return threads, nil
}

func (s *PostgresStore) DeleteTelegramThread(ctx context.Context, olderThan time.Time) error {
	if olderThan.IsZero() {
		return nil
	}

	err := s.gdb.WithContext(ctx).
		Where("updated < ?", olderThan).
		Delete(&types.TelegramThread{}).Error

	if err != nil {
		return err
	}

	return nil
}
