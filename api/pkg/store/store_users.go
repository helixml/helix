package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"

	"gorm.io/gorm"
)

type GetUserQuery struct {
	ID    string
	Email string
}

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

// GetUser retrieves a user by ID
func (s *PostgresStore) GetUser(ctx context.Context, q *GetUserQuery) (*types.User, error) {
	if q.ID == "" && q.Email == "" {
		return nil, fmt.Errorf("userID or email cannot be empty")
	}

	query := s.gdb.WithContext(ctx)

	if q.ID != "" {
		query = query.Where("id = ?", q.ID)
	}
	if q.Email != "" {
		query = query.Where("email = ?", q.Email)
	}

	var user types.User
	err := query.First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}

// CreateUser creates a new user
func (s *PostgresStore) CreateUser(ctx context.Context, user *types.User) (*types.User, error) {
	if user.ID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}

	err := s.gdb.WithContext(ctx).Create(user).Error
	if err != nil {
		return nil, err
	}
	return user, nil
}

// UpdateUser updates an existing user
func (s *PostgresStore) UpdateUser(ctx context.Context, user *types.User) (*types.User, error) {
	if user.ID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}

	err := s.gdb.WithContext(ctx).Save(user).Error
	if err != nil {
		return nil, err
	}
	return user, nil
}

// DeleteUser deletes a user by ID
func (s *PostgresStore) DeleteUser(ctx context.Context, userID string) error {
	if userID == "" {
		return fmt.Errorf("userID cannot be empty")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.User{ID: userID}).Error
	if err != nil {
		return err
	}
	return nil
}

func (s *PostgresStore) ListUsers(ctx context.Context, query *ListUsersQuery) ([]*types.User, error) {
	var users []*types.User
	db := s.gdb.WithContext(ctx)

	if query != nil {
		if query.TokenType != "" {
			db = db.Where("token_type = ?", query.TokenType)
		}
		if query.Admin {
			db = db.Where("admin = ?", true)
		}
		if query.Type != "" {
			db = db.Where("type = ?", query.Type)
		}
		if query.Email != "" {
			db = db.Where("email = ?", query.Email)
		}
		if query.Username != "" {
			db = db.Where("username = ?", query.Username)
		}
	}

	err := db.Find(&users).Error
	if err != nil {
		return nil, err
	}
	return users, nil
}

// SearchUsers searches for users with partial matching on email, name, and username
func (s *PostgresStore) SearchUsers(ctx context.Context, query *SearchUsersQuery) ([]*types.User, int64, error) {
	var users []*types.User
	var total int64

	if query.Query == "" {
		return nil, 0, fmt.Errorf("query cannot be empty")
	}

	// Start with a base query
	db := s.gdb.WithContext(ctx).Model(&types.User{})

	// Filter users by organization membership if organization ID is provided
	if query.OrganizationID != "" {
		db = db.Joins("JOIN organization_memberships ON organization_memberships.user_id = users.id").
			Where("organization_memberships.organization_id = ?", query.OrganizationID)
	}

	// Apply filters for partial matching
	if query.Query != "" {
		db = db.Where("(email ILIKE ? OR full_name ILIKE ? OR username ILIKE ?)",
			"%"+query.Query+"%",
			"%"+query.Query+"%",
			"%"+query.Query+"%")
	}

	// Count total matching records before applying pagination
	err := db.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if query != nil && query.Limit > 0 {
		db = db.Limit(query.Limit)
		if query.Offset > 0 {
			db = db.Offset(query.Offset)
		}
	}

	// Execute the query
	err = db.Debug().Distinct().Find(&users).Error
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (s *PostgresStore) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	err := s.gdb.WithContext(ctx).Model(&types.User{}).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}
