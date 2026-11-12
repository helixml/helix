package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"

	"gorm.io/gorm"
)

type GetUserQuery struct {
	ID    string
	Email string
	// DesktopIdentity selects the Helix Desktop auto-login account (the
	// oldest, should there ever be more than one).
	DesktopIdentity bool
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

	// Track whether we actually need to write. EnsureUserMeta is called from auth
	// middleware on every request, so an unconditional UPDATE here turns every API
	// call into two DB round-trips for no gain.
	dirty := false

	// Ensure existing user has a slug, or regenerate if it looks like a UUID
	shouldRegenerateSlug := false
	if existing.Slug == "" {
		shouldRegenerateSlug = true
	} else {
		// Check if the slug looks like a UUID (contains hyphens and is 36 chars, or is the user ID)
		// This catches cases where the slug was generated from the user ID before FullName was available
		if len(existing.Slug) == 36 && strings.Count(existing.Slug, "-") == 4 {
			shouldRegenerateSlug = true
		} else if existing.Slug == strings.ToLower(existing.ID) || existing.Slug == strings.ReplaceAll(strings.ToLower(existing.ID), "-", "") {
			shouldRegenerateSlug = true
		}
	}

	if shouldRegenerateSlug {
		oldSlug := existing.Slug
		newSlug := s.generateUserSlug(ctx, existing.ID)
		if newSlug != oldSlug {
			existing.Slug = newSlug
			dirty = true
			log.Info().
				Str("user_id", existing.ID).
				Str("old_slug", oldSlug).
				Str("new_slug", newSlug).
				Msg("regenerating user_meta slug with proper name")
		}
	}

	// Merge any new config values from the parameter
	if user.Config.StripeCustomerID != "" && existing.Config.StripeCustomerID != user.Config.StripeCustomerID {
		existing.Config.StripeCustomerID = user.Config.StripeCustomerID
		dirty = true
	}
	if user.Config.StripeSubscriptionID != "" && existing.Config.StripeSubscriptionID != user.Config.StripeSubscriptionID {
		existing.Config.StripeSubscriptionID = user.Config.StripeSubscriptionID
		dirty = true
	}
	if user.Config.StripeSubscriptionActive && !existing.Config.StripeSubscriptionActive {
		existing.Config.StripeSubscriptionActive = user.Config.StripeSubscriptionActive
		dirty = true
	}

	if !dirty {
		return existing, nil
	}
	return s.UpdateUserMeta(ctx, *existing)
}

// GetUser retrieves a user by ID
func (s *PostgresStore) GetUser(ctx context.Context, q *GetUserQuery) (*types.User, error) {
	if q.ID == "" && q.Email == "" && !q.DesktopIdentity {
		return nil, fmt.Errorf("userID or email cannot be empty")
	}

	query := s.gdb.WithContext(ctx)

	if q.DesktopIdentity {
		query = query.Where("desktop_identity = ?", true).Order("created_at ASC")
	}

	if q.ID != "" {
		query = query.Where("id = ?", q.ID)
	}
	var user types.User
	if q.Email != "" {
		// Emails are matched case-insensitively (as invitations are), so an
		// address cannot be registered twice in different cases. Rows created
		// before that rule may differ only in case: an exact match wins, so
		// such an account keeps resolving to itself.
		exact := query.Session(&gorm.Session{}).Where("email = ?", strings.TrimSpace(q.Email))
		if err := exact.Order("created_at ASC").First(&user).Error; err == nil {
			return &user, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		query = query.Where("LOWER(email) = ?", NormalizeEmail(q.Email)).Order("created_at ASC")
	}

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
// RevokeUserAPIKeys deletes every API key the user owns, including org-,
// project- and session-scoped keys. Authentication looks keys up on every
// request, so they stop working as soon as this returns.
func (s *PostgresStore) RevokeUserAPIKeys(ctx context.Context, userID string) error {
	if userID == "" {
		return fmt.Errorf("userID cannot be empty")
	}
	return s.gdb.WithContext(ctx).Where("owner = ?", userID).Delete(&types.ApiKey{}).Error
}

// DeleteUser deletes a user and, in one transaction, the data that belongs to
// the user alone: credentials (API keys, login sessions, OAuth and git
// provider connections, agent subscriptions), personal agents and their
// triggers, personal secrets and provider endpoints, memories, the personal
// wallet, and every organization, team and access-grant membership.
//
// Resources owned by an organization are kept even when the user created
// them; removing the memberships revokes the user's access to them. Personal
// projects, repositories, chat sessions and files need runtime teardown and
// are removed by the caller before this runs. The users row is soft-deleted.
func (s *PostgresStore) DeleteUser(ctx context.Context, userID string) error {
	if userID == "" {
		return fmt.Errorf("userID cannot be empty")
	}

	personal := "(organization_id = '' OR organization_id IS NULL)"
	return s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		personalApps := tx.Model(&types.App{}).Select("id").Where("owner = ? AND "+personal, userID)
		steps := []struct {
			what string
			run  func() error
		}{
			{"api keys", func() error { return tx.Where("owner = ?", userID).Delete(&types.ApiKey{}).Error }},
			{"login sessions", func() error { return tx.Where("user_id = ?", userID).Delete(&types.UserSession{}).Error }},
			{"oauth connections", func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&types.OAuthConnection{}).Error
			}},
			{"oauth request tokens", func() error {
				return tx.Where("user_id = ?", userID).Delete(&types.OAuthRequestToken{}).Error
			}},
			{"git provider connections", func() error {
				return tx.Unscoped().Where("user_id = ?", userID).Delete(&types.GitProviderConnection{}).Error
			}},
			{"claude subscriptions", func() error {
				return tx.Where("owner_id = ? AND owner_type = ?", userID, types.OwnerTypeUser).Delete(&types.ClaudeSubscription{}).Error
			}},
			{"codex subscriptions", func() error {
				return tx.Where("owner_id = ? AND owner_type = ?", userID, types.OwnerTypeUser).Delete(&types.CodexSubscription{}).Error
			}},
			{"triggers", func() error {
				return tx.Where("app_id IN (?) OR (owner = ? AND "+personal+")", personalApps, userID).
					Delete(&types.TriggerConfiguration{}).Error
			}},
			{"secrets", func() error {
				return tx.Where("owner = ? AND owner_type = ? AND (project_id = '' OR project_id IS NULL) AND "+
					"(app_id = '' OR app_id IS NULL OR app_id IN (?))", userID, types.OwnerTypeUser, personalApps).
					Delete(&types.Secret{}).Error
			}},
			{"provider endpoints", func() error {
				return tx.Where("owner = ? AND owner_type = ?", userID, types.OwnerTypeUser).Delete(&types.ProviderEndpoint{}).Error
			}},
			{"memories", func() error { return tx.Where("user_id = ?", userID).Delete(&types.Memory{}).Error }},
			{"agents", func() error { return tx.Where("owner = ? AND "+personal, userID).Delete(&types.App{}).Error }},
			{"wallet", func() error { return tx.Where("user_id = ?", userID).Delete(&types.Wallet{}).Error }},
			{"access grant role bindings", func() error {
				return tx.Where("access_grant_id IN (?)",
					tx.Model(&types.AccessGrant{}).Select("id").Where("user_id = ?", userID),
				).Delete(&types.AccessGrantRoleBinding{}).Error
			}},
			{"access grants", func() error { return tx.Where("user_id = ?", userID).Delete(&types.AccessGrant{}).Error }},
			{"team memberships", func() error { return tx.Where("user_id = ?", userID).Delete(&types.TeamMembership{}).Error }},
			{"organization memberships", func() error {
				return tx.Where("user_id = ?", userID).Delete(&types.OrganizationMembership{}).Error
			}},
			{"user meta", func() error { return tx.Where("id = ?", userID).Delete(&types.UserMeta{}).Error }},
			{"user", func() error { return tx.Delete(&types.User{ID: userID}).Error }},
		}
		for _, step := range steps {
			if err := step.run(); err != nil {
				return fmt.Errorf("delete user %s: %w", step.what, err)
			}
		}
		return nil
	})
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
		if query.Query != "" {
			// Unified free-text search across email, username, and full_name.
			// Overrides the separate Email/Username filters.
			like := "%" + query.Query + "%"
			db = db.Where("email ILIKE ? OR username ILIKE ? OR full_name ILIKE ?", like, like, like)
		} else {
			if query.Email != "" {
				if strings.Contains(query.Email, "@") {
					// Full email address - exact match (case-insensitive)
					db = db.Where("LOWER(email) = LOWER(?)", query.Email)
				} else {
					// Domain only - filter by email domain
					db = db.Where("email ILIKE ?", "%@"+query.Email)
				}
			}
			if query.Username != "" {
				// Support ILIKE matching for username
				db = db.Where("username ILIKE ?", "%"+query.Username+"%")
			}
		}
		if query.Waitlisted != nil {
			db = db.Where("waitlisted = ?", *query.Waitlisted)
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

// TouchUserLastSeen writes the last_seen_at column for a user without touching
// the rest of the record. Callers are responsible for throttling; the store
// just does the write.
func (s *PostgresStore) TouchUserLastSeen(ctx context.Context, userID string, at time.Time) error {
	if userID == "" {
		return fmt.Errorf("userID cannot be empty")
	}
	return s.gdb.WithContext(ctx).
		Model(&types.User{}).
		Where("id = ?", userID).
		Update("last_seen_at", at).
		Error
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
		// Check if slug conflicts with existing user slug (but not this user's own slug)
		var existingUser types.UserMeta
		userErr := s.gdb.WithContext(ctx).Where("slug = ? AND id != ?", slug, userID).First(&existingUser).Error

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
