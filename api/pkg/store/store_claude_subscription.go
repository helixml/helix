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
