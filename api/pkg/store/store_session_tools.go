package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateSessionToolBinding(ctx context.Context, sessionID, toolID string) error {
	if sessionID == "" {
		return fmt.Errorf("session id not specified")
	}

	if toolID == "" {
		return fmt.Errorf("tool id not specified")
	}

	err := s.gdb.WithContext(ctx).Create(&types.SessionToolBinding{
		SessionID: sessionID,
		ToolID:    toolID,
	}).Error
	if err != nil {
		return err
	}
	return nil
}

func (s *PostgresStore) ListSessionTools(ctx context.Context, sessionID string) ([]*types.Tool, error) {
	var binding types.SessionToolBinding

	err := s.gdb.WithContext(ctx).
		Preload("Tools").
		Where(&types.SessionToolBinding{
			SessionID: sessionID,
		}).First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return []*types.Tool{}, nil
		}
		return nil, err
	}

	return binding.Tools, nil
}

func (s *PostgresStore) DeleteSessionToolBinding(ctx context.Context, sessionID, toolID string) error {
	err := s.gdb.WithContext(ctx).Delete(&types.SessionToolBinding{
		SessionID: sessionID,
		ToolID:    toolID,
	}).Error
	if err != nil {
		return err
	}

	return nil
}
