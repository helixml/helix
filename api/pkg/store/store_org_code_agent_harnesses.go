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

func (s *PostgresStore) ListOrgCodeAgentHarnesses(ctx context.Context, orgID string) ([]*types.OrgCodeAgentHarness, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}

	var harnesses []*types.OrgCodeAgentHarness
	if err := s.gdb.WithContext(ctx).
		Where("organization_id = ?", orgID).
		Order("runtime ASC").
		Find(&harnesses).Error; err != nil {
		return nil, err
	}
	return harnesses, nil
}

func (s *PostgresStore) GetOrgCodeAgentHarness(ctx context.Context, orgID string, runtime types.CodeAgentRuntime) (*types.OrgCodeAgentHarness, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}
	if runtime == "" {
		return nil, fmt.Errorf("runtime not specified")
	}

	var harness types.OrgCodeAgentHarness
	if err := s.gdb.WithContext(ctx).
		Where("organization_id = ? AND runtime = ?", orgID, runtime).
		First(&harness).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &harness, nil
}

// UpsertOrgCodeAgentHarnesses updates only the supplied harnesses. A settings
// switch can therefore save one row without replacing the rest of the policy.
func (s *PostgresStore) UpsertOrgCodeAgentHarnesses(ctx context.Context, orgID, actingUserID string, updates []types.OrgCodeAgentHarnessUpdate) ([]*types.OrgCodeAgentHarness, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}
	if len(updates) == 0 {
		return s.ListOrgCodeAgentHarnesses(ctx, orgID)
	}
	for _, update := range updates {
		if !types.IsSelectableCodeAgentRuntime(update.Runtime) {
			return nil, fmt.Errorf("unsupported code agent runtime %q", update.Runtime)
		}
	}

	now := time.Now()
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, update := range updates {
			row := types.OrgCodeAgentHarness{
				ID:             system.GenerateID(),
				Created:        now,
				Updated:        now,
				CreatedBy:      actingUserID,
				UpdatedBy:      actingUserID,
				OrganizationID: orgID,
				Runtime:        update.Runtime,
				Enabled:        update.Enabled,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "organization_id"}, {Name: "runtime"}},
				DoUpdates: clause.Assignments(map[string]any{
					"enabled":    row.Enabled,
					"updated":    now,
					"updated_by": actingUserID,
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ListOrgCodeAgentHarnesses(ctx, orgID)
}
