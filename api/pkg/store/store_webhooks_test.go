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

func TestEnqueueWebhookEventTxMatchesScopeAndFilter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.WebhookEndpoint{}, &types.WebhookEvent{}, &types.WebhookDelivery{}))
	now := time.Now().UTC()
	endpoints := []*types.WebhookEndpoint{
		{ID: "org", OrganizationID: "org-1", Events: []string{types.WebhookEventSpecTaskStatusChanged}, Status: types.WebhookEndpointStatusActive, URL: "https://example.com", SecretEncrypted: "x", SecretPreview: "test", CreatedBy: "u", UpdatedBy: "u", CreatedAt: now, UpdatedAt: now},
		{ID: "project", OrganizationID: "org-1", ProjectID: "project-1", Events: []string{"*"}, Status: types.WebhookEndpointStatusActive, URL: "https://example.com", SecretEncrypted: "x", SecretPreview: "test", CreatedBy: "u", UpdatedBy: "u", CreatedAt: now, UpdatedAt: now},
		{ID: "other-project", OrganizationID: "org-1", ProjectID: "project-2", Events: []string{"*"}, Status: types.WebhookEndpointStatusActive, URL: "https://example.com", SecretEncrypted: "x", SecretPreview: "test", CreatedBy: "u", UpdatedBy: "u", CreatedAt: now, UpdatedAt: now},
		{ID: "wrong-event", OrganizationID: "org-1", Events: []string{types.WebhookEventArtifactPublished}, Status: types.WebhookEndpointStatusActive, URL: "https://example.com", SecretEncrypted: "x", SecretPreview: "test", CreatedBy: "u", UpdatedBy: "u", CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, db.Create(endpoints).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return enqueueWebhookEventTx(tx, types.WebhookEventSpecTaskStatusChanged, "org-1", "project-1", types.SpecTaskWebhookData{SpecTaskID: "task-1"})
	}))
	var events []types.WebhookEvent
	var deliveries []types.WebhookDelivery
	require.NoError(t, db.Find(&events).Error)
	require.NoError(t, db.Order("endpoint_id").Find(&deliveries).Error)
	require.Len(t, events, 1)
	require.Len(t, deliveries, 2)
	require.Equal(t, "org", deliveries[0].EndpointID)
	require.Equal(t, "project", deliveries[1].EndpointID)
	require.Equal(t, events[0].ID, deliveries[0].EventID)
}

func TestClaimWebhookDeliveriesReclaimsExpiredLease(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.WebhookDelivery{}))
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	require.NoError(t, db.Create([]*types.WebhookDelivery{
		{ID: "pending", EventID: "e1", EndpointID: "a", Status: types.WebhookDeliveryStatusPending, NextAttemptAt: now.Add(-time.Second)},
		{ID: "expired", EventID: "e2", EndpointID: "a", Status: types.WebhookDeliveryStatusProcessing, NextAttemptAt: now.Add(-time.Second), LockedUntil: &expired},
		{ID: "locked", EventID: "e3", EndpointID: "a", Status: types.WebhookDeliveryStatusProcessing, NextAttemptAt: now.Add(-time.Second), LockedUntil: &future},
	}).Error)
	st := &PostgresStore{gdb: db}
	claimed, err := st.ClaimWebhookDeliveries(context.Background(), now, now.Add(time.Minute), 10)
	require.NoError(t, err)
	require.Len(t, claimed, 2)
}
