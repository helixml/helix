package store

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// ListOrgCodeAgentProviders returns every configured runtime for an org. Absent
// runtimes are simply not returned — callers treat a missing row as disabled
// rather than the store inventing defaults it would then have to keep in sync
// with types.SelectableCodeAgentRuntimes.
func (s *PostgresStore) ListOrgCodeAgentProviders(ctx context.Context, orgID string) ([]*types.OrgCodeAgentProvider, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}

	var providers []*types.OrgCodeAgentProvider
	err := s.gdb.WithContext(ctx).
		Where("organization_id = ?", orgID).
		Order("runtime ASC, name ASC").
		Find(&providers).Error
	if err != nil {
		return nil, err
	}
	return providers, nil
}

// GetOrgCodeAgentProvider returns one runtime's row, or ErrNotFound.
func (s *PostgresStore) GetOrgCodeAgentProvider(ctx context.Context, orgID string, runtime types.CodeAgentRuntime, name string) (*types.OrgCodeAgentProvider, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}
	if runtime == "" {
		return nil, fmt.Errorf("runtime not specified")
	}

	var provider types.OrgCodeAgentProvider
	err := s.gdb.WithContext(ctx).
		Where("organization_id = ? AND runtime = ? AND name = ?", orgID, runtime, name).
		First(&provider).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &provider, nil
}

// UpsertOrgCodeAgentProviders writes the given runtimes in one transaction.
//
// It is an upsert on (organization_id, runtime, name) rather than a
// delete-and-insert so a partial save — the UI toggling one row — cannot wipe
// the rest of the org's configuration if the caller sends a short list.
//
// deletes remove flavour rows. A built-in row (empty name) is never deleted
// here; the caller rejects that, because the harness list must stay complete.
func (s *PostgresStore) UpsertOrgCodeAgentProviders(ctx context.Context, orgID string, actingUserID string, updates []types.OrgCodeAgentProviderUpdate, deletes []types.OrgCodeAgentProviderRef) ([]*types.OrgCodeAgentProvider, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}
	if len(updates) == 0 && len(deletes) == 0 {
		return s.ListOrgCodeAgentProviders(ctx, orgID)
	}

	for _, update := range updates {
		if !types.IsSelectableCodeAgentRuntime(update.Runtime) {
			return nil, fmt.Errorf("unsupported code agent runtime %q", update.Runtime)
		}
		if update.CredentialType == types.CodeAgentCredentialTypeSubscription &&
			!update.Runtime.SupportsSubscriptionCredentials() {
			return nil, fmt.Errorf("runtime %q cannot authenticate with a subscription", update.Runtime)
		}
	}

	now := time.Now()
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, update := range updates {
			row := types.OrgCodeAgentProvider{
				ID:                 system.GenerateID(),
				Created:            now,
				Updated:            now,
				CreatedBy:          actingUserID,
				UpdatedBy:          actingUserID,
				OrganizationID:     orgID,
				Runtime:            update.Runtime,
				Name:               update.Name,
				Enabled:            update.Enabled,
				CredentialType:     update.CredentialType,
				ProviderEndpointID: update.ProviderEndpointID,
				DefaultModel:       update.DefaultModel,
			}
			// On conflict keep the original id/created/created_by so the row's
			// provenance survives an edit.
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "organization_id"}, {Name: "runtime"}, {Name: "name"}},
				DoUpdates: clause.Assignments(map[string]any{
					"enabled":              row.Enabled,
					"credential_type":      row.CredentialType,
					"provider_endpoint_id": row.ProviderEndpointID,
					"default_model":        row.DefaultModel,
					"updated":              now,
					"updated_by":           actingUserID,
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		for _, ref := range deletes {
			if ref.Name == "" {
				continue
			}
			if err := tx.Where("organization_id = ? AND runtime = ? AND name = ?", orgID, ref.Runtime, ref.Name).
				Delete(&types.OrgCodeAgentProvider{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.ListOrgCodeAgentProviders(ctx, orgID)
}

// DeleteOrgCodeAgentProviders removes every row for an org. Used when an
// organization is deleted.
func (s *PostgresStore) DeleteOrgCodeAgentProviders(ctx context.Context, orgID string) error {
	if orgID == "" {
		return fmt.Errorf("organization_id not specified")
	}
	return s.gdb.WithContext(ctx).
		Where("organization_id = ?", orgID).
		Delete(&types.OrgCodeAgentProvider{}).Error
}
