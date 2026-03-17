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

		HuggingFaceToken:            "old-token",
		KoditEnrichmentProvider:     "old-kodit-provider",
		KoditEnrichmentModel:        "old-kodit-model",
		RAGEmbeddingsProvider:       "old-rag-provider",
		RAGEmbeddingsModel:          "old-rag-model",
		MaxConcurrentDesktops:       1,
		ProvidersManagementEnabled:  false,
		EnforceQuotas:               false,
		OptimusReasoningModelProvider:      "old-reasoning-provider",
		OptimusReasoningModel:              "old-reasoning-model",
		OptimusReasoningModelEffort:        "old-reasoning-effort",
		OptimusGenerationModelProvider:     "old-generation-provider",
		OptimusGenerationModel:             "old-generation-model",
		OptimusSmallReasoningModelProvider: "old-small-reasoning-provider",
		OptimusSmallReasoningModel:         "old-small-reasoning-model",
		OptimusSmallReasoningModelEffort:   "old-small-reasoning-effort",
		OptimusSmallGenerationModelProvider: "old-small-generation-provider",
		OptimusSmallGenerationModel:         "old-small-generation-model",
	}
	require.NoError(t, store.gdb.WithContext(ctx).Create(seed).Error)

	req := &types.SystemSettingsRequest{
		HuggingFaceToken:            strPtr("new-token"),
		KoditEnrichmentProvider:     strPtr("new-kodit-provider"),
		KoditEnrichmentModel:        strPtr("new-kodit-model"),
		RAGEmbeddingsProvider:       strPtr("new-rag-provider"),
		RAGEmbeddingsModel:          strPtr("new-rag-model"),
		MaxConcurrentDesktops:       intPtr(25),
		ProvidersManagementEnabled:  boolPtr(true),
		EnforceQuotas:               boolPtr(true),
		OptimusReasoningModelProvider:      strPtr("new-reasoning-provider"),
		OptimusReasoningModel:              strPtr("new-reasoning-model"),
		OptimusReasoningModelEffort:        strPtr("new-reasoning-effort"),
		OptimusGenerationModelProvider:     strPtr("new-generation-provider"),
		OptimusGenerationModel:             strPtr("new-generation-model"),
		OptimusSmallReasoningModelProvider: strPtr("new-small-reasoning-provider"),
		OptimusSmallReasoningModel:         strPtr("new-small-reasoning-model"),
		OptimusSmallReasoningModelEffort:   strPtr("new-small-reasoning-effort"),
		OptimusSmallGenerationModelProvider: strPtr("new-small-generation-provider"),
		OptimusSmallGenerationModel:         strPtr("new-small-generation-model"),
	}

	updated, err := store.UpdateSystemSettings(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, updated)

	require.Equal(t, "new-token", updated.HuggingFaceToken)
	require.Equal(t, "new-kodit-provider", updated.KoditEnrichmentProvider)
	require.Equal(t, "new-kodit-model", updated.KoditEnrichmentModel)
	require.Equal(t, "new-rag-provider", updated.RAGEmbeddingsProvider)
	require.Equal(t, "new-rag-model", updated.RAGEmbeddingsModel)
	require.Equal(t, 25, updated.MaxConcurrentDesktops)
	require.True(t, updated.ProvidersManagementEnabled)
	require.True(t, updated.EnforceQuotas)
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

		HuggingFaceToken:            "seed-token",
		KoditEnrichmentProvider:     "seed-kodit-provider",
		KoditEnrichmentModel:        "seed-kodit-model",
		RAGEmbeddingsProvider:       "seed-rag-provider",
		RAGEmbeddingsModel:          "seed-rag-model",
		MaxConcurrentDesktops:       10,
		ProvidersManagementEnabled:  true,
		EnforceQuotas:               false,
		OptimusReasoningModelProvider:      "seed-reasoning-provider",
		OptimusReasoningModel:              "seed-reasoning-model",
		OptimusReasoningModelEffort:        "seed-reasoning-effort",
		OptimusGenerationModelProvider:     "seed-generation-provider",
		OptimusGenerationModel:             "seed-generation-model",
		OptimusSmallReasoningModelProvider: "seed-small-reasoning-provider",
		OptimusSmallReasoningModel:         "seed-small-reasoning-model",
		OptimusSmallReasoningModelEffort:   "seed-small-reasoning-effort",
		OptimusSmallGenerationModelProvider: "seed-small-generation-provider",
		OptimusSmallGenerationModel:         "seed-small-generation-model",
	}
	require.NoError(t, store.gdb.WithContext(ctx).Create(seed).Error)

	req := &types.SystemSettingsRequest{
		HuggingFaceToken:            strPtr("updated-token"),
		MaxConcurrentDesktops:       intPtr(33),
		OptimusReasoningModel:       strPtr("updated-reasoning-model"),
		OptimusSmallGenerationModel: strPtr("updated-small-generation-model"),
	}

	updated, err := store.UpdateSystemSettings(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, updated)

	require.Equal(t, "updated-token", updated.HuggingFaceToken)
	require.Equal(t, 33, updated.MaxConcurrentDesktops)
	require.Equal(t, "updated-reasoning-model", updated.OptimusReasoningModel)
	require.Equal(t, "updated-small-generation-model", updated.OptimusSmallGenerationModel)

	require.Equal(t, "seed-kodit-provider", updated.KoditEnrichmentProvider)
	require.Equal(t, "seed-kodit-model", updated.KoditEnrichmentModel)
	require.Equal(t, "seed-rag-provider", updated.RAGEmbeddingsProvider)
	require.Equal(t, "seed-rag-model", updated.RAGEmbeddingsModel)
	require.True(t, updated.ProvidersManagementEnabled)
	require.False(t, updated.EnforceQuotas)
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
		HuggingFaceToken:       strPtr("created-token"),
		MaxConcurrentDesktops:  intPtr(7),
		EnforceQuotas:          boolPtr(true),
		OptimusReasoningModel:  strPtr("created-reasoning-model"),
		OptimusGenerationModel: strPtr("created-generation-model"),
	}

	updated, err := store.UpdateSystemSettings(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, updated)

	require.Equal(t, types.SystemSettingsID, updated.ID)
	require.Equal(t, "created-token", updated.HuggingFaceToken)
	require.Equal(t, 7, updated.MaxConcurrentDesktops)
	require.True(t, updated.EnforceQuotas)
	require.Equal(t, "created-reasoning-model", updated.OptimusReasoningModel)
	require.Equal(t, "created-generation-model", updated.OptimusGenerationModel)
	require.False(t, updated.Created.IsZero())
	require.False(t, updated.Updated.IsZero())

	var count int64
	require.NoError(t, store.gdb.WithContext(ctx).Model(&types.SystemSettings{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
