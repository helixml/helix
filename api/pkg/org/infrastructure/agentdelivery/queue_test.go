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
