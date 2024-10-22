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
		// Check if a secret with the same name already exists for this owner
		var existingSecret types.Secret
		if err := tx.Where("owner = ? AND name = ?", secret.Owner, secret.Name).First(&existingSecret).Error; err == nil {
			return fmt.Errorf("a secret with the name '%s' already exists for this owner", secret.Name)
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
	if q.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	var secrets []*types.Secret
	err := s.gdb.WithContext(ctx).Where(&types.Secret{
		Owner:     q.Owner,
		OwnerType: q.OwnerType,
	}).Find(&secrets).Error
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
