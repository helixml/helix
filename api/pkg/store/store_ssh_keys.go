package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// CreateSSHKey creates a new SSH key record
func (s *PostgresStore) CreateSSHKey(ctx context.Context, key *types.SSHKey) (*types.SSHKey, error) {
	if key.ID == "" {
		key.ID = "sshkey_" + system.GenerateUUID()
	}
	key.Created = time.Now()

	err := s.gdb.WithContext(ctx).Create(key).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH key: %w", err)
	}

	return key, nil
}

// GetSSHKey retrieves an SSH key by ID
func (s *PostgresStore) GetSSHKey(ctx context.Context, id string) (*types.SSHKey, error) {
	var key types.SSHKey
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&key).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get SSH key: %w", err)
	}

	return &key, nil
}

// ListSSHKeys lists all SSH keys for a user
func (s *PostgresStore) ListSSHKeys(ctx context.Context, userID string) ([]*types.SSHKey, error) {
	var keys []*types.SSHKey
	err := s.gdb.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created DESC").
		Find(&keys).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list SSH keys: %w", err)
	}

	return keys, nil
}

// UpdateSSHKeyLastUsed updates the last_used timestamp for an SSH key
func (s *PostgresStore) UpdateSSHKeyLastUsed(ctx context.Context, id string) error {
	err := s.gdb.WithContext(ctx).
		Model(&types.SSHKey{}).
		Where("id = ?", id).
		Update("last_used", time.Now()).Error
	if err != nil {
		return fmt.Errorf("failed to update SSH key last_used: %w", err)
	}

	return nil
}

// DeleteSSHKey deletes an SSH key by ID
func (s *PostgresStore) DeleteSSHKey(ctx context.Context, id string) error {
	result := s.gdb.WithContext(ctx).Delete(&types.SSHKey{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete SSH key: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}
