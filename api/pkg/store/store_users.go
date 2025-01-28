package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"

	"gorm.io/gorm"
)

func (s *PostgresStore) GetUserMeta(ctx context.Context, userID string) (*types.UserMeta, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}

	var user types.UserMeta

	err := s.gdb.WithContext(ctx).Where("id = ?", userID).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (s *PostgresStore) CreateUserMeta(ctx context.Context, user types.UserMeta) (*types.UserMeta, error) {
	if user.ID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}

	err := s.gdb.WithContext(ctx).Create(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *PostgresStore) UpdateUserMeta(ctx context.Context, user types.UserMeta) (*types.UserMeta, error) {
	if user.ID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}

	err := s.gdb.WithContext(ctx).Save(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *PostgresStore) EnsureUserMeta(ctx context.Context, user types.UserMeta) (*types.UserMeta, error) {
	existing, err := s.GetUserMeta(ctx, user.ID)
	if err != nil || existing == nil {
		return s.CreateUserMeta(ctx, user)
	}
	return s.UpdateUserMeta(ctx, user)
}
