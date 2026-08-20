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

func (s *PostgresStore) CreateClaudeSubscription(ctx context.Context, sub *types.ClaudeSubscription) (*types.ClaudeSubscription, error) {
	if sub.ID == "" {
		sub.ID = system.GenerateClaudeSubscriptionID()
	}
	if sub.OwnerID == "" {
		return nil, fmt.Errorf("owner_id not specified")
	}
	if sub.OwnerType == "" {
		return nil, fmt.Errorf("owner_type not specified")
	}
	if sub.EncryptedCredentials == "" {
		return nil, fmt.Errorf("encrypted_credentials not specified")
	}
	if sub.CreatedBy == "" {
		return nil, fmt.Errorf("created_by not specified")
	}

	sub.Created = time.Now()
	sub.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Create(sub).Error
	if err != nil {
		return nil, err
	}
	return s.GetClaudeSubscription(ctx, sub.ID)
}

func (s *PostgresStore) GetClaudeSubscription(ctx context.Context, id string) (*types.ClaudeSubscription, error) {
	var sub types.ClaudeSubscription
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&sub).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sub, nil
}

func (s *PostgresStore) GetClaudeSubscriptionForOwner(ctx context.Context, ownerID string, ownerType types.OwnerType) (*types.ClaudeSubscription, error) {
	var sub types.ClaudeSubscription
	err := s.gdb.WithContext(ctx).
		Where("owner_id = ? AND owner_type = ?", ownerID, ownerType).
		Order("created DESC").
		First(&sub).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sub, nil
}

func (s *PostgresStore) UpdateClaudeSubscription(ctx context.Context, sub *types.ClaudeSubscription) (*types.ClaudeSubscription, error) {
	if sub.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	sub.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Save(sub).Error
	if err != nil {
		return nil, err
	}
	return s.GetClaudeSubscription(ctx, sub.ID)
}

// UpdateClaudeSubscriptionCredentialsIfNewer writes rotated credentials only when
// they are newer than what is stored, and touches only the credential columns.
//
// Three writers race over one row: this API's background refresher, a container
// pushing credentials it refreshed itself, and the status/identity writers. A
// full-row Save from any of them reverts the others — and because Anthropic
// rotates the refresh token on every use, reverting a credential resurrects a
// refresh token that is already dead, permanently bricking the subscription.
// The refreshed_at predicate makes the older write lose instead. This mirrors
// UpdateCodexSubscriptionCredentialsIfNewer.
//
// Status is deliberately not touched: a working refresh proves the refresh
// endpoint accepts the token, not that /v1/messages will. The liveness probe
// owns Status.
func (s *PostgresStore) UpdateClaudeSubscriptionCredentialsIfNewer(ctx context.Context, id, encryptedCredentials string, expiresAt, refreshTokenExpiresAt, refreshedAt time.Time) (bool, error) {
	if id == "" {
		return false, fmt.Errorf("id not specified")
	}
	updates := map[string]interface{}{
		"encrypted_credentials": encryptedCredentials,
		"last_refreshed_at":     refreshedAt,
		"updated":               time.Now(),
	}
	// A credential with no stated expiry must not blank a known one.
	if !expiresAt.IsZero() {
		updates["access_token_expires_at"] = expiresAt
	}
	if !refreshTokenExpiresAt.IsZero() {
		updates["refresh_token_expires_at"] = refreshTokenExpiresAt
	}
	result := s.gdb.WithContext(ctx).Model(&types.ClaudeSubscription{}).
		Where("id = ? AND (last_refreshed_at IS NULL OR last_refreshed_at < ?)", id, refreshedAt).
		Updates(updates)
	return result.RowsAffected == 1, result.Error
}

// UpdateClaudeSubscriptionStatus persists the outcome of a liveness probe and
// the identity it discovered, without touching encrypted_credentials.
//
// The probe plus profile fetch can take ~18s, during which a container may push
// refreshed credentials. Writing the whole row back afterwards would revert
// them; restricting the write to these columns cannot.
func (s *PostgresStore) UpdateClaudeSubscriptionStatus(ctx context.Context, sub *types.ClaudeSubscription) error {
	if sub == nil || sub.ID == "" {
		return fmt.Errorf("id not specified")
	}
	return s.gdb.WithContext(ctx).Model(&types.ClaudeSubscription{}).
		Where("id = ?", sub.ID).
		Updates(map[string]interface{}{
			"status":                 sub.Status,
			"last_error":             sub.LastError,
			"last_validated_at":      sub.LastValidatedAt,
			"subscription_type":      sub.SubscriptionType,
			"rate_limit_tier":        sub.RateLimitTier,
			"account_email":          sub.AccountEmail,
			"account_display_name":   sub.AccountDisplayName,
			"claude_organization_id": sub.ClaudeOrganizationID,
			"updated":                time.Now(),
		}).Error
}

func (s *PostgresStore) DeleteClaudeSubscription(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}
	return s.gdb.WithContext(ctx).Delete(&types.ClaudeSubscription{ID: id}).Error
}

func (s *PostgresStore) ListClaudeSubscriptions(ctx context.Context, ownerID string) ([]*types.ClaudeSubscription, error) {
	var subs []*types.ClaudeSubscription
	query := s.gdb.WithContext(ctx)
	if ownerID != "" {
		query = query.Where("owner_id = ?", ownerID)
	}
	err := query.Order("created DESC").Find(&subs).Error
	if err != nil {
		return nil, err
	}
	return subs, nil
}

// GetSessionClaudeSubscription resolves the Claude subscription that should
// authenticate a session's agent.
//
// When the session names a credential owner (an orchestrator dispatching work on
// a human's behalf — see SpecTask.CredentialOwnerID), that user's own
// subscription is used, but ONLY if they have delegated it to this organization.
// The delegation check is the consent gate: without it, anyone who can create a
// session could name any user as credential owner and spend their Claude quota.
//
// A named-but-undelegated (or missing) credential owner falls back to the
// session owner's normal resolution rather than failing, so a revoked delegation
// degrades to previous behaviour instead of bricking the agent.
func (s *PostgresStore) GetSessionClaudeSubscription(ctx context.Context, session *types.Session) (*types.ClaudeSubscription, error) {
	if session == nil {
		return nil, ErrNotFound
	}
	if owner := session.Metadata.CredentialOwnerID; owner != "" && owner != session.Owner {
		sub, err := s.GetClaudeSubscriptionForOwner(ctx, owner, types.OwnerTypeUser)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if err == nil && subscriptionDelegatedTo(sub, session.OrganizationID) {
			return sub, nil
		}
	}
	return s.GetEffectiveClaudeSubscription(ctx, session.Owner, session.OrganizationID)
}

// subscriptionDelegatedTo reports whether sub's owner has authorised agents in
// orgID to authenticate with it on their behalf.
func subscriptionDelegatedTo(sub *types.ClaudeSubscription, orgID string) bool {
	if sub == nil || orgID == "" {
		return false
	}
	for _, id := range sub.DelegatedOrgIDs {
		if id == orgID {
			return true
		}
	}
	return false
}

// GetEffectiveClaudeSubscription returns the active Claude subscription for a user.
// It checks user-level first (takes priority), then falls back to org-level.
func (s *PostgresStore) GetEffectiveClaudeSubscription(ctx context.Context, userID, orgID string) (*types.ClaudeSubscription, error) {
	// Check user-level subscription first
	sub, err := s.GetClaudeSubscriptionForOwner(ctx, userID, types.OwnerTypeUser)
	if err == nil {
		return sub, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// Fall back to org-level subscription
	if orgID != "" {
		sub, err = s.GetClaudeSubscriptionForOwner(ctx, orgID, types.OwnerTypeOrg)
		if err == nil {
			return sub, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}

	return nil, ErrNotFound
}
