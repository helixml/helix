package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func newTestStoreWithPubSub(t *testing.T) *PostgresStore {
	t.Helper()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	return &PostgresStore{pubsub: ps}
}

// collectEvents subscribes with the given filter and returns a function
// to retrieve all received events (thread-safe).
func collectEvents(t *testing.T, store *PostgresStore, filter *StoreEventSubscriptionFilter) (getEvents func() []*StoreEvent, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	var mu sync.Mutex
	var events []*StoreEvent

	sub, err := store.subscribeStoreEvents(ctx, filter, func(event *StoreEvent) error {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
		return nil
	})
	require.NoError(t, err)

	// Wait for NATS subscription to be established
	time.Sleep(100 * time.Millisecond)

	getEvents = func() []*StoreEvent {
		mu.Lock()
		defer mu.Unlock()
		out := make([]*StoreEvent, len(events))
		copy(out, events)
		return out
	}

	cleanup = func() {
		_ = sub.Unsubscribe()
	}

	return getEvents, cleanup
}

func publishSession(t *testing.T, store *PostgresStore, op StoreEventOperation, session *types.Session) {
	t.Helper()
	err := store.publishStoreEvent(context.Background(), op, session)
	require.NoError(t, err)
}

func publishSpecTask(t *testing.T, store *PostgresStore, op StoreEventOperation, task *types.SpecTask) {
	t.Helper()
	err := store.publishStoreEvent(context.Background(), op, task)
	require.NoError(t, err)
}

// waitForEvents waits until the expected number of events is received or times out.
func waitForEvents(t *testing.T, getEvents func() []*StoreEvent, expected int) []*StoreEvent {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		events := getEvents()
		if len(events) >= expected {
			return events
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d events, got %d", expected, len(events))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// ---------- Session Event Tests ----------

func TestSessionEvents_SubscribeByOrganization(t *testing.T) {
	store := newTestStoreWithPubSub(t)

	getEvents, cleanup := collectEvents(t, store, &StoreEventSubscriptionFilter{
		ResourceType:   StoreEventResourceTypeSession,
		OrganizationID: "org-1",
	})
	defer cleanup()

	// Should match: org-1
	publishSession(t, store, StoreEventOperationCreated, &types.Session{
		ID: "ses-1", OrganizationID: "org-1", ProjectID: "proj-1",
	})
	// Should NOT match: org-2
	publishSession(t, store, StoreEventOperationCreated, &types.Session{
		ID: "ses-2", OrganizationID: "org-2", ProjectID: "proj-2",
	})
	// Should match: org-1, different project
	publishSession(t, store, StoreEventOperationUpdated, &types.Session{
		ID: "ses-3", OrganizationID: "org-1", ProjectID: "proj-2",
	})

	events := waitForEvents(t, getEvents, 2)
	require.Len(t, events, 2)
	require.Equal(t, "ses-1", events[0].ResourceID)
	require.Equal(t, StoreEventOperationCreated, events[0].Operation)
	require.Equal(t, "ses-3", events[1].ResourceID)
	require.Equal(t, StoreEventOperationUpdated, events[1].Operation)
}

func TestSessionEvents_SubscribeAllProjects(t *testing.T) {
	store := newTestStoreWithPubSub(t)

	// Subscribe to all session events for org-1, no project filter
	getEvents, cleanup := collectEvents(t, store, &StoreEventSubscriptionFilter{
		ResourceType:   StoreEventResourceTypeSession,
		OrganizationID: "org-1",
	})
	defer cleanup()

	publishSession(t, store, StoreEventOperationCreated, &types.Session{
		ID: "ses-1", OrganizationID: "org-1", ProjectID: "proj-1",
	})
	publishSession(t, store, StoreEventOperationCreated, &types.Session{
		ID: "ses-2", OrganizationID: "org-1", ProjectID: "proj-2",
	})
	publishSession(t, store, StoreEventOperationCreated, &types.Session{
		ID: "ses-3", OrganizationID: "org-1", ProjectID: "proj-3",
	})

	events := waitForEvents(t, getEvents, 3)
	require.Len(t, events, 3)
	require.Equal(t, "ses-1", events[0].ResourceID)
	require.Equal(t, "ses-2", events[1].ResourceID)
	require.Equal(t, "ses-3", events[2].ResourceID)
}

func TestSessionEvents_SubscribeSpecificProject(t *testing.T) {
	store := newTestStoreWithPubSub(t)

	getEvents, cleanup := collectEvents(t, store, &StoreEventSubscriptionFilter{
		ResourceType:   StoreEventResourceTypeSession,
		OrganizationID: "org-1",
		ProjectID:      "proj-1",
	})
	defer cleanup()

	// Should match: org-1, proj-1
	publishSession(t, store, StoreEventOperationCreated, &types.Session{
		ID: "ses-1", OrganizationID: "org-1", ProjectID: "proj-1",
	})
	// Should NOT match: org-1, proj-2
	publishSession(t, store, StoreEventOperationCreated, &types.Session{
		ID: "ses-2", OrganizationID: "org-1", ProjectID: "proj-2",
	})
	// Should match: org-1, proj-1 again
	publishSession(t, store, StoreEventOperationUpdated, &types.Session{
		ID: "ses-3", OrganizationID: "org-1", ProjectID: "proj-1",
	})

	events := waitForEvents(t, getEvents, 2)
	require.Len(t, events, 2)
	require.Equal(t, "ses-1", events[0].ResourceID)
	require.Equal(t, "ses-3", events[1].ResourceID)
}

func TestSessionEvents_SubscribeSingleSession(t *testing.T) {
	store := newTestStoreWithPubSub(t)

	getEvents, cleanup := collectEvents(t, store, &StoreEventSubscriptionFilter{
		ResourceType:   StoreEventResourceTypeSession,
		OrganizationID: "org-1",
		ResourceID:     "ses-target",
	})
	defer cleanup()

	// Should NOT match: different session ID
	publishSession(t, store, StoreEventOperationCreated, &types.Session{
		ID: "ses-other", OrganizationID: "org-1", ProjectID: "proj-1",
	})
	// Should match: target session
	publishSession(t, store, StoreEventOperationUpdated, &types.Session{
		ID: "ses-target", OrganizationID: "org-1", ProjectID: "proj-1",
	})
	// Should match: target session, different operation
	publishSession(t, store, StoreEventOperationDeleted, &types.Session{
		ID: "ses-target", OrganizationID: "org-1", ProjectID: "proj-1",
	})

	events := waitForEvents(t, getEvents, 2)
	require.Len(t, events, 2)
	require.Equal(t, "ses-target", events[0].ResourceID)
	require.Equal(t, StoreEventOperationUpdated, events[0].Operation)
	require.Equal(t, "ses-target", events[1].ResourceID)
	require.Equal(t, StoreEventOperationDeleted, events[1].Operation)
}

func TestSessionEvents_UnmarshalResource(t *testing.T) {
	store := newTestStoreWithPubSub(t)

	getEvents, cleanup := collectEvents(t, store, &StoreEventSubscriptionFilter{
		ResourceType:   StoreEventResourceTypeSession,
		OrganizationID: "org-1",
	})
	defer cleanup()

	publishSession(t, store, StoreEventOperationCreated, &types.Session{
		ID: "ses-1", OrganizationID: "org-1", ProjectID: "proj-1", Name: "Test Session",
	})

	events := waitForEvents(t, getEvents, 1)

	var session types.Session
	err := events[0].UnmarshalResource(&session)
	require.NoError(t, err)
	require.Equal(t, "ses-1", session.ID)
	require.Equal(t, "Test Session", session.Name)
	require.Equal(t, "org-1", session.OrganizationID)
	require.Equal(t, "proj-1", session.ProjectID)
}

func TestSessionEvents_DoesNotReceiveSpecTaskEvents(t *testing.T) {
	store := newTestStoreWithPubSub(t)

	getEvents, cleanup := collectEvents(t, store, &StoreEventSubscriptionFilter{
		ResourceType:   StoreEventResourceTypeSession,
		OrganizationID: "org-1",
	})
	defer cleanup()

	// Publish a spec task event for the same org — session subscriber should not see it
	publishSpecTask(t, store, StoreEventOperationCreated, &types.SpecTask{
		ID: "task-1", OrganizationID: "org-1", ProjectID: "proj-1",
	})
	// Publish a matching session event
	publishSession(t, store, StoreEventOperationCreated, &types.Session{
		ID: "ses-1", OrganizationID: "org-1", ProjectID: "proj-1",
	})

	events := waitForEvents(t, getEvents, 1)
	// Give extra time to ensure the spec task event doesn't arrive
	time.Sleep(200 * time.Millisecond)
	events = getEvents()
	require.Len(t, events, 1)
	require.Equal(t, StoreEventResourceTypeSession, events[0].ResourceType)
	require.Equal(t, "ses-1", events[0].ResourceID)
}

// ---------- Spec Task Event Tests ----------

func TestSpecTaskEvents_SubscribeByOrganization(t *testing.T) {
	store := newTestStoreWithPubSub(t)

	getEvents, cleanup := collectEvents(t, store, &StoreEventSubscriptionFilter{
		ResourceType:   StoreEventResourceTypeSpecTask,
		OrganizationID: "org-1",
	})
	defer cleanup()

	// Should match: org-1
	publishSpecTask(t, store, StoreEventOperationCreated, &types.SpecTask{
		ID: "task-1", OrganizationID: "org-1", ProjectID: "proj-1",
	})
	// Should NOT match: org-2
	publishSpecTask(t, store, StoreEventOperationCreated, &types.SpecTask{
		ID: "task-2", OrganizationID: "org-2", ProjectID: "proj-2",
	})
	// Should match: org-1, different project
	publishSpecTask(t, store, StoreEventOperationUpdated, &types.SpecTask{
		ID: "task-3", OrganizationID: "org-1", ProjectID: "proj-2",
	})

	events := waitForEvents(t, getEvents, 2)
	require.Len(t, events, 2)
	require.Equal(t, "task-1", events[0].ResourceID)
	require.Equal(t, StoreEventOperationCreated, events[0].Operation)
	require.Equal(t, "task-3", events[1].ResourceID)
	require.Equal(t, StoreEventOperationUpdated, events[1].Operation)
}

func TestSpecTaskEvents_SubscribeAllProjects(t *testing.T) {
	store := newTestStoreWithPubSub(t)

	// Subscribe to all spec task events for org-1, no project filter
	getEvents, cleanup := collectEvents(t, store, &StoreEventSubscriptionFilter{
		ResourceType:   StoreEventResourceTypeSpecTask,
		OrganizationID: "org-1",
	})
	defer cleanup()

	publishSpecTask(t, store, StoreEventOperationCreated, &types.SpecTask{
		ID: "task-1", OrganizationID: "org-1", ProjectID: "proj-1",
	})
	publishSpecTask(t, store, StoreEventOperationCreated, &types.SpecTask{
		ID: "task-2", OrganizationID: "org-1", ProjectID: "proj-2",
	})
	publishSpecTask(t, store, StoreEventOperationCreated, &types.SpecTask{
		ID: "task-3", OrganizationID: "org-1", ProjectID: "proj-3",
	})

	events := waitForEvents(t, getEvents, 3)
	require.Len(t, events, 3)
	require.Equal(t, "task-1", events[0].ResourceID)
	require.Equal(t, "task-2", events[1].ResourceID)
	require.Equal(t, "task-3", events[2].ResourceID)
}

func TestSpecTaskEvents_SubscribeSpecificProject(t *testing.T) {
	store := newTestStoreWithPubSub(t)

	getEvents, cleanup := collectEvents(t, store, &StoreEventSubscriptionFilter{
		ResourceType:   StoreEventResourceTypeSpecTask,
		OrganizationID: "org-1",
		ProjectID:      "proj-1",
	})
	defer cleanup()

	// Should match: org-1, proj-1
	publishSpecTask(t, store, StoreEventOperationCreated, &types.SpecTask{
		ID: "task-1", OrganizationID: "org-1", ProjectID: "proj-1",
	})
	// Should NOT match: org-1, proj-2
	publishSpecTask(t, store, StoreEventOperationCreated, &types.SpecTask{
		ID: "task-2", OrganizationID: "org-1", ProjectID: "proj-2",
	})
	// Should match: org-1, proj-1 again
	publishSpecTask(t, store, StoreEventOperationUpdated, &types.SpecTask{
		ID: "task-3", OrganizationID: "org-1", ProjectID: "proj-1",
	})

	events := waitForEvents(t, getEvents, 2)
	require.Len(t, events, 2)
	require.Equal(t, "task-1", events[0].ResourceID)
	require.Equal(t, "task-3", events[1].ResourceID)
}

func TestSpecTaskEvents_SubscribeSingleTask(t *testing.T) {
	store := newTestStoreWithPubSub(t)

	getEvents, cleanup := collectEvents(t, store, &StoreEventSubscriptionFilter{
		ResourceType:   StoreEventResourceTypeSpecTask,
		OrganizationID: "org-1",
		ResourceID:     "task-target",
	})
	defer cleanup()

	// Should NOT match: different task ID
	publishSpecTask(t, store, StoreEventOperationCreated, &types.SpecTask{
		ID: "task-other", OrganizationID: "org-1", ProjectID: "proj-1",
	})
	// Should match: target task
	publishSpecTask(t, store, StoreEventOperationUpdated, &types.SpecTask{
		ID: "task-target", OrganizationID: "org-1", ProjectID: "proj-1",
	})
	// Should match: target task, different operation
	publishSpecTask(t, store, StoreEventOperationDeleted, &types.SpecTask{
		ID: "task-target", OrganizationID: "org-1", ProjectID: "proj-1",
	})

	events := waitForEvents(t, getEvents, 2)
	require.Len(t, events, 2)
	require.Equal(t, "task-target", events[0].ResourceID)
	require.Equal(t, StoreEventOperationUpdated, events[0].Operation)
	require.Equal(t, "task-target", events[1].ResourceID)
	require.Equal(t, StoreEventOperationDeleted, events[1].Operation)
}

func TestSpecTaskEvents_UnmarshalResource(t *testing.T) {
	store := newTestStoreWithPubSub(t)

	getEvents, cleanup := collectEvents(t, store, &StoreEventSubscriptionFilter{
		ResourceType:   StoreEventResourceTypeSpecTask,
		OrganizationID: "org-1",
	})
	defer cleanup()

	publishSpecTask(t, store, StoreEventOperationCreated, &types.SpecTask{
		ID: "task-1", OrganizationID: "org-1", ProjectID: "proj-1", Name: "Build feature X",
	})

	events := waitForEvents(t, getEvents, 1)

	var task types.SpecTask
	err := events[0].UnmarshalResource(&task)
	require.NoError(t, err)
	require.Equal(t, "task-1", task.ID)
	require.Equal(t, "Build feature X", task.Name)
	require.Equal(t, "org-1", task.OrganizationID)
	require.Equal(t, "proj-1", task.ProjectID)
}

func TestSpecTaskEvents_DoesNotReceiveSessionEvents(t *testing.T) {
	store := newTestStoreWithPubSub(t)

	getEvents, cleanup := collectEvents(t, store, &StoreEventSubscriptionFilter{
		ResourceType:   StoreEventResourceTypeSpecTask,
		OrganizationID: "org-1",
	})
	defer cleanup()

	// Publish a session event for the same org — spec task subscriber should not see it
	publishSession(t, store, StoreEventOperationCreated, &types.Session{
		ID: "ses-1", OrganizationID: "org-1", ProjectID: "proj-1",
	})
	// Publish a matching spec task event
	publishSpecTask(t, store, StoreEventOperationCreated, &types.SpecTask{
		ID: "task-1", OrganizationID: "org-1", ProjectID: "proj-1",
	})

	events := waitForEvents(t, getEvents, 1)
	time.Sleep(200 * time.Millisecond)
	events = getEvents()
	require.Len(t, events, 1)
	require.Equal(t, StoreEventResourceTypeSpecTask, events[0].ResourceType)
	require.Equal(t, "task-1", events[0].ResourceID)
}

// ---------- Filter Unit Tests ----------

func TestStoreEventSubscriptionFilter_Matches(t *testing.T) {
	event := &StoreEvent{
		ResourceType:   StoreEventResourceTypeSession,
		ResourceID:     "ses-1",
		OrganizationID: "org-1",
		ProjectID:      "proj-1",
	}

	t.Run("NilFilter_MatchesEverything", func(t *testing.T) {
		var f *StoreEventSubscriptionFilter
		require.True(t, f.Matches(event))
	})

	t.Run("EmptyFilter_MatchesEverything", func(t *testing.T) {
		f := &StoreEventSubscriptionFilter{}
		require.True(t, f.Matches(event))
	})

	t.Run("ResourceType_Match", func(t *testing.T) {
		f := &StoreEventSubscriptionFilter{ResourceType: StoreEventResourceTypeSession}
		require.True(t, f.Matches(event))
	})

	t.Run("ResourceType_NoMatch", func(t *testing.T) {
		f := &StoreEventSubscriptionFilter{ResourceType: StoreEventResourceTypeSpecTask}
		require.False(t, f.Matches(event))
	})

	t.Run("ResourceID_Match", func(t *testing.T) {
		f := &StoreEventSubscriptionFilter{ResourceID: "ses-1"}
		require.True(t, f.Matches(event))
	})

	t.Run("ResourceID_NoMatch", func(t *testing.T) {
		f := &StoreEventSubscriptionFilter{ResourceID: "ses-999"}
		require.False(t, f.Matches(event))
	})

	t.Run("OrganizationID_Match", func(t *testing.T) {
		f := &StoreEventSubscriptionFilter{OrganizationID: "org-1"}
		require.True(t, f.Matches(event))
	})

	t.Run("OrganizationID_NoMatch", func(t *testing.T) {
		f := &StoreEventSubscriptionFilter{OrganizationID: "org-999"}
		require.False(t, f.Matches(event))
	})

	t.Run("ProjectID_Match", func(t *testing.T) {
		f := &StoreEventSubscriptionFilter{ProjectID: "proj-1"}
		require.True(t, f.Matches(event))
	})

	t.Run("ProjectID_NoMatch", func(t *testing.T) {
		f := &StoreEventSubscriptionFilter{ProjectID: "proj-999"}
		require.False(t, f.Matches(event))
	})

	t.Run("CombinedFilters_AllMatch", func(t *testing.T) {
		f := &StoreEventSubscriptionFilter{
			ResourceType:   StoreEventResourceTypeSession,
			ResourceID:     "ses-1",
			OrganizationID: "org-1",
			ProjectID:      "proj-1",
		}
		require.True(t, f.Matches(event))
	})

	t.Run("CombinedFilters_OneDoesNotMatch", func(t *testing.T) {
		f := &StoreEventSubscriptionFilter{
			ResourceType:   StoreEventResourceTypeSession,
			ResourceID:     "ses-1",
			OrganizationID: "org-1",
			ProjectID:      "proj-wrong",
		}
		require.False(t, f.Matches(event))
	})
}

func TestBuildStoreEvent(t *testing.T) {
	t.Run("Session", func(t *testing.T) {
		event, err := buildStoreEvent(StoreEventOperationCreated, &types.Session{
			ID: "ses-1", OrganizationID: "org-1", ProjectID: "proj-1", Name: "My Session",
		})
		require.NoError(t, err)
		require.Equal(t, StoreEventOperationCreated, event.Operation)
		require.Equal(t, StoreEventResourceTypeSession, event.ResourceType)
		require.Equal(t, "ses-1", event.ResourceID)
		require.Equal(t, "org-1", event.OrganizationID)
		require.Equal(t, "proj-1", event.ProjectID)
		require.False(t, event.OccurredAt.IsZero())

		var session types.Session
		err = event.UnmarshalResource(&session)
		require.NoError(t, err)
		require.Equal(t, "My Session", session.Name)
	})

	t.Run("SpecTask", func(t *testing.T) {
		event, err := buildStoreEvent(StoreEventOperationUpdated, &types.SpecTask{
			ID: "task-1", OrganizationID: "org-1", ProjectID: "proj-1", Name: "Fix bug",
		})
		require.NoError(t, err)
		require.Equal(t, StoreEventOperationUpdated, event.Operation)
		require.Equal(t, StoreEventResourceTypeSpecTask, event.ResourceType)
		require.Equal(t, "task-1", event.ResourceID)
		require.Equal(t, "org-1", event.OrganizationID)
		require.Equal(t, "proj-1", event.ProjectID)

		var task types.SpecTask
		err = event.UnmarshalResource(&task)
		require.NoError(t, err)
		require.Equal(t, "Fix bug", task.Name)
	})

	t.Run("NilSession_Error", func(t *testing.T) {
		_, err := buildStoreEvent(StoreEventOperationCreated, (*types.Session)(nil))
		require.Error(t, err)
	})

	t.Run("NilSpecTask_Error", func(t *testing.T) {
		_, err := buildStoreEvent(StoreEventOperationCreated, (*types.SpecTask)(nil))
		require.Error(t, err)
	})

	t.Run("UnsupportedType_Error", func(t *testing.T) {
		_, err := buildStoreEvent(StoreEventOperationCreated, "not a resource")
		require.Error(t, err)
	})
}
