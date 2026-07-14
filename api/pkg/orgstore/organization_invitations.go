package orgstore

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
	// AppID — when set, filter invitations to those sent from a specific
	// app's access-management dialog. Used to scope the "pending
	// invitations" list shown next to AccessGrants on a project page.
	AppID string
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
func (s *Store) CreateOrganizationInvitation(ctx context.Context, inv *types.OrganizationInvitation) (*types.OrganizationInvitation, error) {
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

func (s *Store) GetOrganizationInvitation(ctx context.Context, q *GetOrganizationInvitationQuery) (*types.OrganizationInvitation, error) {
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

func (s *Store) ListOrganizationInvitations(ctx context.Context, q *ListOrganizationInvitationsQuery) ([]*types.OrganizationInvitation, error) {
	query := s.gdb.WithContext(ctx).Order("created_at DESC")
	if q != nil {
		if q.OrganizationID != "" {
			query = query.Where("organization_id = ?", q.OrganizationID)
		}
		if q.Email != "" {
			query = query.Where("email = ?", strings.ToLower(strings.TrimSpace(q.Email)))
		}
		if q.AppID != "" {
			query = query.Where("app_id = ?", q.AppID)
		}
	}

	var invitations []*types.OrganizationInvitation
	if err := query.Find(&invitations).Error; err != nil {
		return nil, err
	}
	return invitations, nil
}

func (s *Store) DeleteOrganizationInvitation(ctx context.Context, id string) error {
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
// deletes the invitations. If the invitation was created from a project's
// access-management dialog (AppID set), an AccessGrant for that app is
// also materialised so the invitee shows up in the project's access list
// immediately. All work happens inside a single transaction so a partial
// failure can't leave the invitation in an "accepted but not joined"
// state. Returns the list of memberships that were created.
func (s *Store) ConsumePendingInvitations(ctx context.Context, user *types.User) ([]*types.OrganizationMembership, error) {
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
			membershipAlreadyExisted := err == nil

			if !membershipAlreadyExisted {
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
				created = append(created, membership)
			}

			// Materialise the access grant captured at invite time. We do
			// this even when a membership already existed — the inviter's
			// intent ("grant access to app X") is independent of any
			// other org-join path the user happened to take in the
			// meantime. Skip duplicates by leaning on the unique check
			// inside the per-grant logic; here we just no-op if a grant
			// already exists for this (resource, user) pair.
			if inv.AppID != "" {
				if err := createInvitationAccessGrant(tx, inv, user.ID); err != nil {
					return err
				}
			}

			if err := tx.Where("id = ?", inv.ID).Delete(&types.OrganizationInvitation{}).Error; err != nil {
				return fmt.Errorf("delete invitation %s: %w", inv.ID, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// createInvitationAccessGrant runs inside ConsumePendingInvitations'
// transaction. It looks up the org's roles by name (since invitations
// store role *names*, not IDs — names are stable across role rebuilds)
// and creates an AccessGrant + role bindings for the new user. If a grant
// already exists for this (resource, user), we silently skip — re-running
// the consumer should be idempotent.
func createInvitationAccessGrant(tx *gorm.DB, inv *types.OrganizationInvitation, userID string) error {
	var existing types.AccessGrant
	dupErr := tx.Where("resource_id = ? AND user_id = ?", inv.AppID, userID).First(&existing).Error
	if dupErr == nil {
		return nil
	}
	if !errors.Is(dupErr, gorm.ErrRecordNotFound) {
		return fmt.Errorf("check existing access grant: %w", dupErr)
	}

	// Resolve role names → role rows. Invitations store names rather
	// than IDs because the inviter picks from a stable taxonomy
	// ("app_user", "admin") that survives role-table rebuilds. Names
	// the org doesn't define are simply skipped — better to grant the
	// known subset than to fail the whole accept flow.
	var roles []*types.Role
	if len(inv.GrantRoles) > 0 {
		if err := tx.Where("organization_id = ? AND name IN ?", inv.OrganizationID, []string(inv.GrantRoles)).
			Find(&roles).Error; err != nil {
			return fmt.Errorf("look up grant roles: %w", err)
		}
	}

	grant := &types.AccessGrant{
		ID:             system.GenerateAccessGrantID(),
		OrganizationID: inv.OrganizationID,
		ResourceID:     inv.AppID,
		UserID:         userID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := tx.Create(grant).Error; err != nil {
		return fmt.Errorf("create access grant: %w", err)
	}
	for _, role := range roles {
		binding := &types.AccessGrantRoleBinding{
			AccessGrantID:  grant.ID,
			RoleID:         role.ID,
			OrganizationID: inv.OrganizationID,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		if err := tx.Create(binding).Error; err != nil {
			return fmt.Errorf("create access grant role binding: %w", err)
		}
	}
	return nil
}
