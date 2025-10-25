package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"

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
		log.Info().
			Str("user_id", existing.ID).
			Str("generated_slug", existing.Slug).
			Msg("updating user_meta with generated slug")
		return s.UpdateUserMeta(ctx, *existing)
	}

	// Merge any new config values from the parameter
	if user.Config.StripeCustomerID != "" {
		existing.Config.StripeCustomerID = user.Config.StripeCustomerID
	}
	if user.Config.StripeSubscriptionID != "" {
		existing.Config.StripeSubscriptionID = user.Config.StripeSubscriptionID
	}
	if user.Config.StripeSubscriptionActive {
		existing.Config.StripeSubscriptionActive = user.Config.StripeSubscriptionActive
	}

	return s.UpdateUserMeta(ctx, *existing)
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

// generateUserSlug creates a URL-friendly slug from a user's name, email, or ID
// Similar to GitHub usernames: lowercase, alphanumeric, hyphens only
func (s *PostgresStore) generateUserSlug(ctx context.Context, userID string) string {
	// Try to get the actual user to extract username/email for better slug
	user, err := s.GetUser(ctx, &GetUserQuery{ID: userID})
	if err != nil {
		log.Warn().Err(err).Str("user_id", userID).Msg("failed to get user for slug generation")
	}

	var baseText string
	if err == nil && user != nil {
		// Prefer full name first (most readable), then username (unless it's an email), then email
		if user.FullName != "" {
			baseText = user.FullName
		} else if user.Username != "" && !strings.Contains(user.Username, "@") {
			// Use username only if it's not an email address
			baseText = user.Username
		} else if user.Email != "" {
			// Extract username part from email (before @)
			parts := strings.Split(user.Email, "@")
			baseText = parts[0]
		}
		log.Debug().
			Str("user_id", userID).
			Str("username", user.Username).
			Str("full_name", user.FullName).
			Str("email", user.Email).
			Str("base_text", baseText).
			Msg("slug generation from user data")
	}

	// Fallback to user ID if we couldn't get better info
	if baseText == "" {
		baseText = userID
		log.Warn().Str("user_id", userID).Msg("slug generation falling back to user ID")
	}

	// Start with the base text
	slug := strings.ToLower(baseText)

	// Remove spaces entirely, replace other non-alphanumeric with hyphens
	slug = strings.ReplaceAll(slug, " ", "")

	// Replace remaining non-alphanumeric characters (except hyphens) with hyphens
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

	// Check for uniqueness against both user_meta slugs and organization names
	baseSlug := slug
	counter := 1
	for {
		// Check if slug conflicts with existing user slug
		var existingUser types.UserMeta
		userErr := s.gdb.WithContext(ctx).Where("slug = ?", slug).First(&existingUser).Error

		// Check if slug conflicts with organization name
		var existingOrg types.Organization
		orgErr := s.gdb.WithContext(ctx).Where("name = ?", slug).First(&existingOrg).Error

		if userErr == gorm.ErrRecordNotFound && orgErr == gorm.ErrRecordNotFound {
			// Slug is unique across both users and orgs
			break
		}
		if (userErr != nil && userErr != gorm.ErrRecordNotFound) ||
		   (orgErr != nil && orgErr != gorm.ErrRecordNotFound) {
			// On error, just use the slug as-is
			break
		}
		// Slug exists, try with counter
		counter++
		slug = fmt.Sprintf("%s-%d", baseSlug, counter)
	}

	log.Info().
		Str("user_id", userID).
		Str("slug", slug).
		Str("base_slug", baseSlug).
		Int("counter", counter-1).
		Msg("generated user slug")

	return slug
}
