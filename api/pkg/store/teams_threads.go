package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateTeamsThread(ctx context.Context, thread *types.TeamsThread) (*types.TeamsThread, error) {
	if thread.ConversationID == "" {
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

func (s *PostgresStore) GetTeamsThread(ctx context.Context, appID, conversationID string) (*types.TeamsThread, error) {
	if appID == "" || conversationID == "" {
		return nil, ErrNotFound
	}

	var thread types.TeamsThread
	err := s.gdb.WithContext(ctx).
		Where("app_id = ? AND conversation_id = ?", appID, conversationID).
		First(&thread).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &thread, nil
}

func (s *PostgresStore) DeleteTeamsThread(ctx context.Context, olderThan time.Time) error {
	if olderThan.IsZero() {
		return nil
	}

	err := s.gdb.WithContext(ctx).
		Where("updated < ?", olderThan).
		Delete(&types.TeamsThread{}).Error

	if err != nil {
		return err
	}

	return nil
}
