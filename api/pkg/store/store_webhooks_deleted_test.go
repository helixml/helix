package store

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A deleted webhook endpoint keeps its row (and deliveries) but is no longer
// returned by Get or List, so every route under it answers 404.
func TestWebhookEndpoints_DeletedAreHidden(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.WebhookEndpoint{}))
	st := &PostgresStore{gdb: db}
	ctx := context.Background()

	now := time.Now().UTC()
	for _, id := range []string{"live", "gone"} {
		require.NoError(t, db.Create(&types.WebhookEndpoint{
			ID: id, OrganizationID: "org-1", Events: []string{"*"}, Status: types.WebhookEndpointStatusActive,
			URL: "https://example.com", SecretEncrypted: "x", SecretPreview: "p", CreatedBy: "u", UpdatedBy: "u",
			CreatedAt: now, UpdatedAt: now,
		}).Error)
	}
	gone, err := st.GetWebhookEndpoint(ctx, "org-1", "gone")
	require.NoError(t, err)
	gone.Status = types.WebhookEndpointStatusDeleted
	require.NoError(t, st.UpdateWebhookEndpointConfig(ctx, gone))

	_, err = st.GetWebhookEndpoint(ctx, "org-1", "gone")
	require.ErrorIs(t, err, ErrNotFound)
	listed, err := st.ListWebhookEndpoints(ctx, "org-1")
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "live", listed[0].ID)

	var count int64
	require.NoError(t, db.Model(&types.WebhookEndpoint{}).Where("id = ?", "gone").Count(&count).Error)
	require.EqualValues(t, 1, count, "the row is kept")
}
