package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// UpsertZedSettingsOverride creates or updates Zed settings overrides for a session
func (s *PostgresStore) UpsertZedSettingsOverride(ctx context.Context, override *types.ZedSettingsOverride) error {
	override.UpdatedAt = time.Now()

	// Use GORM's OnConflict to handle upsert
	result := s.gdb.WithContext(ctx).
		Exec(`
			INSERT INTO zed_settings_overrides (session_id, overrides, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT (session_id)
			DO UPDATE SET overrides = EXCLUDED.overrides, updated_at = EXCLUDED.updated_at
		`, override.SessionID, override.Overrides, override.UpdatedAt)

	if result.Error != nil {
		return fmt.Errorf("failed to upsert Zed settings override: %w", result.Error)
	}

	return nil
}

// GetZedSettingsOverride retrieves Zed settings overrides for a session
func (s *PostgresStore) GetZedSettingsOverride(ctx context.Context, sessionID string) (*types.ZedSettingsOverride, error) {
	var override types.ZedSettingsOverride

	result := s.gdb.WithContext(ctx).
		Where("session_id = ?", sessionID).
		First(&override)

	if result.Error != nil {
		return nil, result.Error
	}

	return &override, nil
}

// DeleteZedSettingsOverride deletes Zed settings overrides for a session
func (s *PostgresStore) DeleteZedSettingsOverride(ctx context.Context, sessionID string) error {
	result := s.gdb.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&types.ZedSettingsOverride{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete Zed settings override: %w", result.Error)
	}

	return nil
}
