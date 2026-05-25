package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

type ListOrganizationInvitationsQuery struct {
	OrganizationID string
	Email          string
}

// ErrInvitationAlreadyExists is returned by CreateOrganizationInvitation
// when a pending invitation already exists for (organization_id, email).
// Callers can recover the existing row via GetOrganizationInvitation and
// treat the create call as a no-op (typical for "resend invite" UX).
var ErrInvitationAlreadyExists = errors.New("invitation already exists")

type GetOrganizationInvitationQuery struct {
	ID             string
	OrganizationID string
	Email          string
}

// CreateOrganizationInvitation persists a pending invitation. Email is
// normalised to lowercase so case-insensitive lookups at registration time
// always hit. We enforce uniqueness (org_id, email) at the application layer
// because empty placeholder rows can otherwise accumulate.
func (s *PostgresStore) CreateOrganizationInvitation(ctx context.Context, inv *types.OrganizationInvitation) (*types.OrganizationInvitation, error) {
	if inv.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}
	if inv.Email == "" {
		return nil, fmt.Errorf("email not specified")
	}

	inv.Email = strings.ToLower(strings.TrimSpace(inv.Email))

	if inv.ID == "" {
		inv.ID = system.GenerateOrgInvitationID()
	}
	if inv.Role == "" {
		inv.Role = types.OrganizationRoleMember
	}
	now := time.Now()
	inv.CreatedAt = now
	inv.UpdatedAt = now

	// Surface duplicates via a sentinel so callers can react (typical UX
	// is "ok, just resend the email"). We don't silently overwrite — that
	// would mask a role-change attempt that the caller should perform
	// explicitly.
	existing, err := s.GetOrganizationInvitation(ctx, &GetOrganizationInvitationQuery{
		OrganizationID: inv.OrganizationID,
		Email:          inv.Email,
	})
	if err == nil && existing != nil {
		return existing, ErrInvitationAlreadyExists
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	if err := s.gdb.WithContext(ctx).Create(inv).Error; err != nil {
		return nil, err
	}
	return inv, nil
}

func (s *PostgresStore) GetOrganizationInvitation(ctx context.Context, q *GetOrganizationInvitationQuery) (*types.OrganizationInvitation, error) {
	if q == nil || (q.ID == "" && (q.OrganizationID == "" || q.Email == "")) {
		return nil, fmt.Errorf("either id or (organization_id, email) must be specified")
	}

	query := s.gdb.WithContext(ctx)
	if q.ID != "" {
		query = query.Where("id = ?", q.ID)
	}
	if q.OrganizationID != "" {
		query = query.Where("organization_id = ?", q.OrganizationID)
	}
	if q.Email != "" {
		query = query.Where("email = ?", strings.ToLower(strings.TrimSpace(q.Email)))
	}

	var inv types.OrganizationInvitation
	err := query.First(&inv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &inv, nil
}

func (s *PostgresStore) ListOrganizationInvitations(ctx context.Context, q *ListOrganizationInvitationsQuery) ([]*types.OrganizationInvitation, error) {
	query := s.gdb.WithContext(ctx).Order("created_at DESC")
	if q != nil {
		if q.OrganizationID != "" {
			query = query.Where("organization_id = ?", q.OrganizationID)
		}
		if q.Email != "" {
			query = query.Where("email = ?", strings.ToLower(strings.TrimSpace(q.Email)))
		}
	}

	var invitations []*types.OrganizationInvitation
	if err := query.Find(&invitations).Error; err != nil {
		return nil, err
	}
	return invitations, nil
}

func (s *PostgresStore) DeleteOrganizationInvitation(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id must be specified")
	}
	res := s.gdb.WithContext(ctx).Where("id = ?", id).Delete(&types.OrganizationInvitation{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ConsumePendingInvitations finds every pending invitation matching the
// user's email, creates the corresponding organization membership, and
// deletes the invitations. All work happens inside a single transaction so
// a failed membership write can't leave the invitation in an "accepted but
// not joined" state. Returns the list of memberships that were created.
func (s *PostgresStore) ConsumePendingInvitations(ctx context.Context, user *types.User) ([]*types.OrganizationMembership, error) {
	if user == nil || user.ID == "" || user.Email == "" {
		return nil, nil
	}

	email := strings.ToLower(strings.TrimSpace(user.Email))

	var created []*types.OrganizationMembership
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var invitations []*types.OrganizationInvitation
		if err := tx.Where("email = ?", email).Find(&invitations).Error; err != nil {
			return fmt.Errorf("list invitations: %w", err)
		}
		now := time.Now()
		for _, inv := range invitations {
			// Skip if a membership already exists (e.g. created via some
			// other path like OIDC domain-join).
			var existing types.OrganizationMembership
			err := tx.Where("organization_id = ? AND user_id = ?", inv.OrganizationID, user.ID).First(&existing).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("check existing membership: %w", err)
			}
			if err == nil {
				if delErr := tx.Where("id = ?", inv.ID).Delete(&types.OrganizationInvitation{}).Error; delErr != nil {
					return fmt.Errorf("delete invitation %s: %w", inv.ID, delErr)
				}
				continue
			}

			role := inv.Role
			if role == "" {
				role = types.OrganizationRoleMember
			}
			membership := &types.OrganizationMembership{
				OrganizationID: inv.OrganizationID,
				UserID:         user.ID,
				Role:           role,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := tx.Create(membership).Error; err != nil {
				return fmt.Errorf("create membership for org %s: %w", inv.OrganizationID, err)
			}
			if err := tx.Where("id = ?", inv.ID).Delete(&types.OrganizationInvitation{}).Error; err != nil {
				return fmt.Errorf("delete invitation %s: %w", inv.ID, err)
			}
			created = append(created, membership)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}
