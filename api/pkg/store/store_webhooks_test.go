package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPreparingSpecTaskPublishesCreatedWebhookOnlyOnTransition(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.SpecTask{},
		&types.WebhookEndpoint{},
		&types.WebhookEvent{},
		&types.WebhookDelivery{},
	))

	now := time.Now().UTC()
	require.NoError(t, db.Create(&types.WebhookEndpoint{
		ID:              "endpoint",
		OrganizationID:  "org-1",
		ProjectID:       "project-1",
		Events:          []string{types.WebhookEventSpecTaskCreated},
		Status:          types.WebhookEndpointStatusActive,
		URL:             "https://example.com",
		SecretEncrypted: "x",
		SecretPreview:   "test",
		CreatedBy:       "user-1",
		UpdatedBy:       "user-1",
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error)

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	st := &PostgresStore{gdb: db, pubsub: ps}
	getEvents, cleanup := collectEvents(t, st, &StoreEventSubscriptionFilter{
		ResourceType: StoreEventResourceTypeSpecTask,
		ResourceID:   "task-1",
	})
	defer cleanup()
	task := &types.SpecTask{
		ID:             "task-1",
		OrganizationID: "org-1",
		ProjectID:      "project-1",
		Status:         types.TaskStatusPreparing,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	require.NoError(t, st.CreateSpecTask(context.Background(), task))

	var count int64
	require.NoError(t, db.Model(&types.WebhookEvent{}).Count(&count).Error)
	require.Zero(t, count)
	time.Sleep(50 * time.Millisecond)
	require.Empty(t, getEvents())

	transitioned, err := st.TransitionSpecTaskStatus(
		context.Background(),
		task.ID,
		[]types.SpecTaskStatus{types.TaskStatusPreparing},
		types.TaskStatusBacklog,
		nil,
	)
	require.NoError(t, err)
	require.True(t, transitioned)

	var event types.WebhookEvent
	require.NoError(t, db.Take(&event).Error)
	require.Equal(t, types.WebhookEventSpecTaskCreated, event.Type)
	var data types.SpecTaskWebhookData
	require.NoError(t, json.Unmarshal(event.Data, &data))
	require.Equal(t, task.ID, data.SpecTaskID)
	require.Equal(t, types.TaskStatusBacklog, data.Status)
	storeEvents := waitForEvents(t, getEvents, 1)
	require.Len(t, storeEvents, 1)
	require.Equal(t, StoreEventOperationCreated, storeEvents[0].Operation)
	var published types.SpecTask
	require.NoError(t, storeEvents[0].UnmarshalResource(&published))
	require.Equal(t, types.TaskStatusBacklog, published.Status)
}

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
