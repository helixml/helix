package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

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

	// Auto-generate slug from user ID if not provided
	if user.Slug == "" {
		user.Slug = s.generateUserSlug(ctx, user.ID)
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

	// Ensure existing user has a slug
	if existing.Slug == "" {
		existing.Slug = s.generateUserSlug(ctx, existing.ID)
		return s.UpdateUserMeta(ctx, *existing)
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

	// Delete all API keys for the user
	err = s.gdb.WithContext(ctx).Where("owner = ?", userID).Delete(&types.ApiKey{}).Error
	if err != nil {
		return err
	}

	// Delete user meta
	err = s.gdb.WithContext(ctx).Delete(&types.UserMeta{
		ID: userID,
	}).Error
	if err != nil {
		return err
	}

	return nil
}

func (s *PostgresStore) ListUsers(ctx context.Context, query *ListUsersQuery) ([]*types.User, int64, error) {
	var users []*types.User
	var total int64
	db := s.gdb.WithContext(ctx).Model(&types.User{})

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
			// Support ILIKE matching for email domain filtering
			if strings.Contains(query.Email, "@") {
				// If it contains @, treat as domain filter
				db = db.Where("email ILIKE ?", "%@"+query.Email)
			} else {
				// Otherwise, exact match
				db = db.Where("email = ?", query.Email)
			}
		}
		if query.Username != "" {
			// Support ILIKE matching for username
			db = db.Where("username ILIKE ?", "%"+query.Username+"%")
		}
	}

	// Count total matching records before applying pagination
	err := db.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if query != nil && query.PerPage > 0 {
		// Enforce maximum page size of 200
		if query.PerPage > 200 {
			query.PerPage = 200
		}
		db = db.Limit(query.PerPage)
		if query.Page > 0 {
			db = db.Offset((query.Page - 1) * query.PerPage)
		}
	}

	// Apply ordering
	orderBy := "created_at DESC"
	if query != nil && query.Order != "" {
		orderBy = query.Order
	}
	db = db.Order(orderBy)

	err = db.Find(&users).Error
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
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

// generateUserSlug creates a URL-friendly slug from a user ID or email
// Similar to GitHub usernames: lowercase, alphanumeric, hyphens only
func (s *PostgresStore) generateUserSlug(ctx context.Context, userID string) string {
	// Start with the user ID
	slug := strings.ToLower(userID)

	// Replace non-alphanumeric characters (except hyphens) with hyphens
	reg := regexp.MustCompile(`[^a-z0-9-]+`)
	slug = reg.ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Collapse multiple consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	slug = reg.ReplaceAllString(slug, "-")

	// Ensure slug is not empty
	if slug == "" {
		slug = "user"
	}

	// Check for uniqueness and append number if needed
	baseSlug := slug
	counter := 1
	for {
		var existing types.UserMeta
		err := s.gdb.WithContext(ctx).Where("slug = ?", slug).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			// Slug is unique
			break
		}
		if err != nil {
			// On error, just use the slug as-is
			break
		}
		// Slug exists, try with counter
		counter++
		slug = fmt.Sprintf("%s-%d", baseSlug, counter)
	}

	return slug
}
