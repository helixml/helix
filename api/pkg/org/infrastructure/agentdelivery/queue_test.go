package agentdelivery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/stretchr/testify/require"
)

func TestQueueRetriesFailedActivation(t *testing.T) {
	n, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	defer n.Close()

	var mu sync.Mutex
	attempts := 0
	completed := make(chan struct{})
	q, err := New(context.Background(), n, func(context.Context, string, orgchart.NodeID, []activation.Trigger) error {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		if attempts == 1 {
			return errors.New("transient")
		}
		close(completed)
		return nil
	}, nil)
	require.NoError(t, err)
	defer q.Close()

	q.Enqueue("org-test", "agent-a", activation.Trigger{Kind: activation.TriggerEvent, EventID: "e-1"})
	select {
	case <-completed:
	case <-time.After(5 * time.Second):
		t.Fatal("activation was not retried")
	}

	mu.Lock()
	require.Equal(t, 2, attempts)
	mu.Unlock()
}

func TestRetryDelay(t *testing.T) {
	require.Equal(t, time.Second, retryDelay(1))
	require.Equal(t, 2*time.Second, retryDelay(2))
	require.Equal(t, 30*time.Minute, retryDelay(12))
	require.Equal(t, 30*time.Minute, retryDelay(^uint64(0)))
}

func TestQueueFansOutPerAgent(t *testing.T) {
	n, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	defer n.Close()

	seen := make(chan string, 2)
	q, err := New(context.Background(), n, func(_ context.Context, _ string, agentID orgchart.NodeID, _ []activation.Trigger) error {
		seen <- string(agentID)
		return nil
	}, nil)
	require.NoError(t, err)
	defer q.Close()

	trigger := activation.Trigger{Kind: activation.TriggerEvent, EventID: "e-1"}
	q.Enqueue("org-test", "agent-a", trigger)
	q.Enqueue("org-test", "agent-b", trigger)

	got := map[string]bool{}
	for range 2 {
		select {
		case id := <-seen:
			got[id] = true
		case <-time.After(5 * time.Second):
			t.Fatal("one of the subscribed agents did not activate")
		}
	}
	require.Equal(t, map[string]bool{"agent-a": true, "agent-b": true}, got)
}

func TestQueueCleanupAgentRemovesConsumerAndPendingMessages(t *testing.T) {
	n, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	defer n.Close()

	seen := make(chan string, 1)
	q, err := New(context.Background(), n, func(_ context.Context, _ string, _ orgchart.NodeID, triggers []activation.Trigger) error {
		seen <- triggers[0].EventID
		return errors.New("retry")
	}, nil)
	require.NoError(t, err)
	defer q.Close()

	q.Enqueue("org-test", "agent-a", activation.Trigger{Kind: activation.TriggerEvent, EventID: "old"})
	require.Equal(t, "old", <-seen)
	require.NoError(t, q.CleanupAgent(context.Background(), "org-test", "agent-a"))

	consumers, err := n.ListDurableConsumers(context.Background(), streamName)
	require.NoError(t, err)
	require.Empty(t, consumers)
	q.Enqueue("org-test", "agent-a", activation.Trigger{Kind: activation.TriggerEvent, EventID: "ignored"})
	consumers, err = n.ListDurableConsumers(context.Background(), streamName)
	require.NoError(t, err)
	require.Empty(t, consumers)

	received := make(chan string, 1)
	sub, err := n.ConsumeDurable(context.Background(), streamName, "probe", subjectFor("org-test", "agent-a"), time.Second, func(msg *pubsub.Message) error {
		received <- string(msg.Data)
		return msg.Ack()
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()
	require.NoError(t, n.PublishDurable(context.Background(), streamName, subjectFor("org-test", "agent-a"), []byte("new")))
	require.Equal(t, "new", <-received)
	require.NoError(t, sub.Unsubscribe())
	require.NoError(t, n.DeleteDurableConsumer(context.Background(), streamName, "probe"))
	require.NoError(t, n.PurgeDurableSubject(context.Background(), streamName, subjectFor("org-test", "agent-a")))

	q.RestoreAgent("org-test", "agent-a")
	q.Enqueue("org-test", "agent-a", activation.Trigger{Kind: activation.TriggerHire, EventID: "recreated"})
	require.Equal(t, "recreated", <-seen)
}

func TestQueueCancelOutstandingAllowsFreshActivation(t *testing.T) {
	n, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	defer n.Close()

	seen := make(chan string, 2)
	q, err := New(context.Background(), n, func(_ context.Context, _ string, _ orgchart.NodeID, triggers []activation.Trigger) error {
		seen <- triggers[0].EventID
		if triggers[0].EventID == "old" {
			return errors.New("unconfigured")
		}
		return nil
	}, nil)
	require.NoError(t, err)
	defer q.Close()

	q.Enqueue("org-test", "agent-a", activation.Trigger{Kind: activation.TriggerEvent, EventID: "old"})
	require.Equal(t, "old", <-seen)
	require.NoError(t, q.CancelOutstanding(context.Background(), "org-test", "agent-a"))

	q.Enqueue("org-test", "agent-a", activation.Trigger{Kind: activation.TriggerManual, EventID: "fresh"})
	select {
	case got := <-seen:
		require.Equal(t, "fresh", got)
	case <-time.After(5 * time.Second):
		t.Fatal("fresh activation remained blocked behind cancelled delivery")
	}
}
