package store

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
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
	var tools []*types.Tool

	// Join tools and session_tool_bindings tables to get all tools that are bound to the session
	err := s.gdb.WithContext(ctx).Model(&types.Tool{}).
		Joins("JOIN session_tool_bindings ON session_tool_bindings.tool_id = tools.id").
		Where("session_tool_bindings.session_id = ?", sessionID).
		Find(&tools).Error
	if err != nil {
		return nil, err
	}

	return tools, nil
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
