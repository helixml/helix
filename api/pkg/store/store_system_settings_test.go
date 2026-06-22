package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSystemSettingsTestStore(t *testing.T) *PostgresStore {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.SystemSettings{}))

	return &PostgresStore{gdb: db}
}

func strPtr(v string) *string { return &v }
func intPtr(v int) *int       { return &v }
func boolPtr(v bool) *bool    { return &v }

func TestUpdateSystemSettings_UpdatesAllFields(t *testing.T) {
	ctx := context.Background()
	store := newSystemSettingsTestStore(t)

	originalUpdated := time.Now().Add(-2 * time.Hour)
	seed := &types.SystemSettings{
		ID:      types.SystemSettingsID,
		Created: originalUpdated.Add(-time.Hour),
		Updated: originalUpdated,

		HuggingFaceToken:                     "old-token",
		KoditEnrichmentProvider:              "old-kodit-provider",
		KoditEnrichmentModel:                 "old-kodit-model",
		ProvidersManagementEnabled:           false,
		EnforceQuotas:                        false,
		SandboxBillingEnabled:                false,
		SandboxHeadlessPriceCreditsPerSecond: 0.01,
		SandboxDesktopPriceCreditsPerSecond:  0.05,
		MaxConcurrentHeadlessSandboxes:       8,
		MaxConcurrentDesktopSandboxes:        4,
		OptimusReasoningModelProvider:        "old-reasoning-provider",
		OptimusReasoningModel:                "old-reasoning-model",
		OptimusReasoningModelEffort:          "old-reasoning-effort",
		OptimusGenerationModelProvider:       "old-generation-provider",
		OptimusGenerationModel:               "old-generation-model",
		OptimusSmallReasoningModelProvider:   "old-small-reasoning-provider",
		OptimusSmallReasoningModel:           "old-small-reasoning-model",
		OptimusSmallReasoningModelEffort:     "old-small-reasoning-effort",
		OptimusSmallGenerationModelProvider:  "old-small-generation-provider",
		OptimusSmallGenerationModel:          "old-small-generation-model",
	}
	require.NoError(t, store.gdb.WithContext(ctx).Create(seed).Error)

	req := &types.SystemSettingsRequest{
		HuggingFaceToken:                     strPtr("new-token"),
		KoditEnrichmentProvider:              strPtr("new-kodit-provider"),
		KoditEnrichmentModel:                 strPtr("new-kodit-model"),
		ProvidersManagementEnabled:           boolPtr(true),
		EnforceQuotas:                        boolPtr(true),
		SandboxBillingEnabled:                boolPtr(true),
		SandboxHeadlessPriceCreditsPerSecond: floatPtr(0.02),
		SandboxDesktopPriceCreditsPerSecond:  floatPtr(0.08),
		MaxConcurrentHeadlessSandboxes:       intPtr(12),
		MaxConcurrentDesktopSandboxes:        intPtr(6),
		OptimusReasoningModelProvider:        strPtr("new-reasoning-provider"),
		OptimusReasoningModel:                strPtr("new-reasoning-model"),
		OptimusReasoningModelEffort:          strPtr("new-reasoning-effort"),
		OptimusGenerationModelProvider:       strPtr("new-generation-provider"),
		OptimusGenerationModel:               strPtr("new-generation-model"),
		OptimusSmallReasoningModelProvider:   strPtr("new-small-reasoning-provider"),
		OptimusSmallReasoningModel:           strPtr("new-small-reasoning-model"),
		OptimusSmallReasoningModelEffort:     strPtr("new-small-reasoning-effort"),
		OptimusSmallGenerationModelProvider:  strPtr("new-small-generation-provider"),
		OptimusSmallGenerationModel:          strPtr("new-small-generation-model"),
	}

	updated, err := store.UpdateSystemSettings(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, updated)

	require.Equal(t, "new-token", updated.HuggingFaceToken)
	require.Equal(t, "new-kodit-provider", updated.KoditEnrichmentProvider)
	require.Equal(t, "new-kodit-model", updated.KoditEnrichmentModel)
	require.True(t, updated.ProvidersManagementEnabled)
	require.True(t, updated.EnforceQuotas)
	require.True(t, updated.SandboxBillingEnabled)
	require.Equal(t, 0.02, updated.SandboxHeadlessPriceCreditsPerSecond)
	require.Equal(t, 0.08, updated.SandboxDesktopPriceCreditsPerSecond)
	require.Equal(t, 12, updated.MaxConcurrentHeadlessSandboxes)
	require.Equal(t, 6, updated.MaxConcurrentDesktopSandboxes)
	require.Equal(t, "new-reasoning-provider", updated.OptimusReasoningModelProvider)
	require.Equal(t, "new-reasoning-model", updated.OptimusReasoningModel)
	require.Equal(t, "new-reasoning-effort", updated.OptimusReasoningModelEffort)
	require.Equal(t, "new-generation-provider", updated.OptimusGenerationModelProvider)
	require.Equal(t, "new-generation-model", updated.OptimusGenerationModel)
	require.Equal(t, "new-small-reasoning-provider", updated.OptimusSmallReasoningModelProvider)
	require.Equal(t, "new-small-reasoning-model", updated.OptimusSmallReasoningModel)
	require.Equal(t, "new-small-reasoning-effort", updated.OptimusSmallReasoningModelEffort)
	require.Equal(t, "new-small-generation-provider", updated.OptimusSmallGenerationModelProvider)
	require.Equal(t, "new-small-generation-model", updated.OptimusSmallGenerationModel)
	require.True(t, updated.Updated.After(originalUpdated))

	var persisted types.SystemSettings
	require.NoError(t, store.gdb.WithContext(ctx).Where("id = ?", types.SystemSettingsID).First(&persisted).Error)
	require.Equal(t, updated.HuggingFaceToken, persisted.HuggingFaceToken)
	require.Equal(t, updated.OptimusSmallGenerationModel, persisted.OptimusSmallGenerationModel)
}

func TestUpdateSystemSettings_PartialUpdateLeavesOtherFieldsUnchanged(t *testing.T) {
	ctx := context.Background()
	store := newSystemSettingsTestStore(t)

	seed := &types.SystemSettings{
		ID:      types.SystemSettingsID,
		Created: time.Now().Add(-2 * time.Hour),
		Updated: time.Now().Add(-time.Hour),

		HuggingFaceToken:                     "seed-token",
		KoditEnrichmentProvider:              "seed-kodit-provider",
		KoditEnrichmentModel:                 "seed-kodit-model",
		ProvidersManagementEnabled:           true,
		EnforceQuotas:                        false,
		SandboxBillingEnabled:                true,
		SandboxHeadlessPriceCreditsPerSecond: 0.03,
		SandboxDesktopPriceCreditsPerSecond:  0.09,
		MaxConcurrentHeadlessSandboxes:       7,
		MaxConcurrentDesktopSandboxes:        3,
		OptimusReasoningModelProvider:        "seed-reasoning-provider",
		OptimusReasoningModel:                "seed-reasoning-model",
		OptimusReasoningModelEffort:          "seed-reasoning-effort",
		OptimusGenerationModelProvider:       "seed-generation-provider",
		OptimusGenerationModel:               "seed-generation-model",
		OptimusSmallReasoningModelProvider:   "seed-small-reasoning-provider",
		OptimusSmallReasoningModel:           "seed-small-reasoning-model",
		OptimusSmallReasoningModelEffort:     "seed-small-reasoning-effort",
		OptimusSmallGenerationModelProvider:  "seed-small-generation-provider",
		OptimusSmallGenerationModel:          "seed-small-generation-model",
	}
	require.NoError(t, store.gdb.WithContext(ctx).Create(seed).Error)

	req := &types.SystemSettingsRequest{
		HuggingFaceToken:            strPtr("updated-token"),
		OptimusReasoningModel:       strPtr("updated-reasoning-model"),
		OptimusSmallGenerationModel: strPtr("updated-small-generation-model"),
	}

	updated, err := store.UpdateSystemSettings(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, updated)

	require.Equal(t, "updated-token", updated.HuggingFaceToken)
	require.Equal(t, "updated-reasoning-model", updated.OptimusReasoningModel)
	require.Equal(t, "updated-small-generation-model", updated.OptimusSmallGenerationModel)

	require.Equal(t, "seed-kodit-provider", updated.KoditEnrichmentProvider)
	require.Equal(t, "seed-kodit-model", updated.KoditEnrichmentModel)
	require.True(t, updated.ProvidersManagementEnabled)
	require.False(t, updated.EnforceQuotas)
	require.True(t, updated.SandboxBillingEnabled)
	require.Equal(t, 0.03, updated.SandboxHeadlessPriceCreditsPerSecond)
	require.Equal(t, 0.09, updated.SandboxDesktopPriceCreditsPerSecond)
	require.Equal(t, 7, updated.MaxConcurrentHeadlessSandboxes)
	require.Equal(t, 3, updated.MaxConcurrentDesktopSandboxes)
	require.Equal(t, "seed-reasoning-provider", updated.OptimusReasoningModelProvider)
	require.Equal(t, "seed-reasoning-effort", updated.OptimusReasoningModelEffort)
	require.Equal(t, "seed-generation-provider", updated.OptimusGenerationModelProvider)
	require.Equal(t, "seed-generation-model", updated.OptimusGenerationModel)
	require.Equal(t, "seed-small-reasoning-provider", updated.OptimusSmallReasoningModelProvider)
	require.Equal(t, "seed-small-reasoning-model", updated.OptimusSmallReasoningModel)
	require.Equal(t, "seed-small-reasoning-effort", updated.OptimusSmallReasoningModelEffort)
	require.Equal(t, "seed-small-generation-provider", updated.OptimusSmallGenerationModelProvider)
}

func TestUpdateSystemSettings_CreatesSystemRecordIfMissing(t *testing.T) {
	ctx := context.Background()
	store := newSystemSettingsTestStore(t)

	req := &types.SystemSettingsRequest{
		HuggingFaceToken:                     strPtr("created-token"),
		EnforceQuotas:                        boolPtr(true),
		SandboxBillingEnabled:                boolPtr(true),
		SandboxHeadlessPriceCreditsPerSecond: floatPtr(0.04),
		MaxConcurrentHeadlessSandboxes:       intPtr(11),
		MaxConcurrentDesktopSandboxes:        intPtr(5),
		OptimusReasoningModel:                strPtr("created-reasoning-model"),
		OptimusGenerationModel:               strPtr("created-generation-model"),
	}

	updated, err := store.UpdateSystemSettings(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, updated)

	require.Equal(t, types.SystemSettingsID, updated.ID)
	require.Equal(t, "created-token", updated.HuggingFaceToken)
	require.True(t, updated.EnforceQuotas)
	require.True(t, updated.SandboxBillingEnabled)
	require.Equal(t, 0.04, updated.SandboxHeadlessPriceCreditsPerSecond)
	require.Equal(t, 11, updated.MaxConcurrentHeadlessSandboxes)
	require.Equal(t, 5, updated.MaxConcurrentDesktopSandboxes)
	require.Equal(t, "created-reasoning-model", updated.OptimusReasoningModel)
	require.Equal(t, "created-generation-model", updated.OptimusGenerationModel)
	require.False(t, updated.Created.IsZero())
	require.False(t, updated.Updated.IsZero())

	var count int64
	require.NoError(t, store.gdb.WithContext(ctx).Model(&types.SystemSettings{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func floatPtr(v float64) *float64 { return &v }

func TestUpdateSystemSettings_RejectsNegativeSandboxPricing(t *testing.T) {
	ctx := context.Background()
	store := newSystemSettingsTestStore(t)

	_, err := store.UpdateSystemSettings(ctx, &types.SystemSettingsRequest{
		SandboxHeadlessPriceCreditsPerSecond: floatPtr(-0.01),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "sandbox headless price")

	_, err = store.UpdateSystemSettings(ctx, &types.SystemSettingsRequest{
		SandboxDesktopPriceCreditsPerSecond: floatPtr(-0.01),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "sandbox desktop price")
}

func TestUpdateSystemSettings_RejectsInvalidSandboxLimits(t *testing.T) {
	ctx := context.Background()
	store := newSystemSettingsTestStore(t)

	_, err := store.UpdateSystemSettings(ctx, &types.SystemSettingsRequest{
		MaxConcurrentHeadlessSandboxes: intPtr(0),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "max concurrent headless sandboxes")

	_, err = store.UpdateSystemSettings(ctx, &types.SystemSettingsRequest{
		MaxConcurrentDesktopSandboxes: intPtr(-1),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "max concurrent desktop sandboxes")
}
