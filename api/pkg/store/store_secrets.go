package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateSecret(ctx context.Context, secret *types.Secret) (*types.Secret, error) {
	if secret.ID == "" {
		secret.ID = system.GenerateSecretID()
	}

	if secret.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	secret.Created = time.Now()
	secret.Updated = secret.Created

	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Uniqueness is scoped per (owner, project_id, app_id, name) so the same
		// name can be reused across different projects or apps for the same owner.
		var existingSecret types.Secret
		if err := tx.Where(
			"owner = ? AND name = ? AND project_id = ? AND app_id = ?",
			secret.Owner, secret.Name, secret.ProjectID, secret.AppID,
		).First(&existingSecret).Error; err == nil {
			scope := "this owner"
			switch {
			case secret.ProjectID != "":
				scope = "this project"
			case secret.AppID != "":
				scope = "this app"
			}
			return fmt.Errorf("a secret with the name '%s' already exists for %s", secret.Name, scope)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// If no existing secret found, create the new one
		return tx.Create(secret).Error
	})
	if err != nil {
		return nil, err
	}
	return s.GetSecret(ctx, secret.ID)
}

func (s *PostgresStore) UpdateSecret(ctx context.Context, secret *types.Secret) (*types.Secret, error) {
	if secret.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if secret.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	secret.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Save(secret).Error
	if err != nil {
		return nil, err
	}
	return s.GetSecret(ctx, secret.ID)
}

func (s *PostgresStore) GetSecret(ctx context.Context, id string) (*types.Secret, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}

	var secret types.Secret
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&secret).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &secret, nil
}

func (s *PostgresStore) ListSecrets(ctx context.Context, q *ListSecretsQuery) ([]*types.Secret, error) {
	if q.Owner == "" && q.ProjectID == "" {
		return nil, fmt.Errorf("owner or project_id must be specified")
	}

	var secrets []*types.Secret
	query := s.gdb.WithContext(ctx)

	if q.Owner != "" {
		query = query.Where("owner = ?", q.Owner)
	}
	if q.OwnerType != "" {
		query = query.Where("owner_type = ?", q.OwnerType)
	}
	if q.ProjectID != "" {
		query = query.Where("project_id = ?", q.ProjectID)
	}
	if q.UserLevelOnly {
		query = query.Where("project_id = ? AND app_id = ?", "", "")
	}

	err := query.Find(&secrets).Error
	if err != nil {
		return nil, err
	}
	return secrets, nil
}

// ListProjectSecrets returns all secrets for a project (by any owner who has access)
func (s *PostgresStore) ListProjectSecrets(ctx context.Context, projectID string) ([]*types.Secret, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project_id not specified")
	}

	var secrets []*types.Secret
	err := s.gdb.WithContext(ctx).Where("project_id = ?", projectID).Find(&secrets).Error
	if err != nil {
		return nil, err
	}
	return secrets, nil
}

func (s *PostgresStore) DeleteSecret(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.Secret{
		ID: id,
	}).Error
	if err != nil {
		return err
	}
	return nil
}
