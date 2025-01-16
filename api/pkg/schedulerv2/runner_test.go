package schedulerv2

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func TestSendToRunner(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl, err := NewRunnerController(context.Background(), &RunnerControllerConfig{
		PubSub: ps,
	})
	require.NoError(t, err)

	mockRunnerID := "test"

	mockRunner, err := ps.SubscribeWithCtx(context.Background(), pubsub.GetRunnerQueue(mockRunnerID), func(ctx context.Context, msg *nats.Msg) error {
		response := &types.Response{
			StatusCode: 200,
			Body:       "test",
		}
		responseBytes, err := json.Marshal(response)
		require.NoError(t, err)
		msg.Respond(responseBytes)
		return nil
	})
	require.NoError(t, err)
	defer mockRunner.Unsubscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	response, err := ctrl.Send(ctx, mockRunnerID, &types.Request{
		Method: "GET",
		URL:    "https://example.com",
		Body:   "{}",
	})
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, 200, response.StatusCode)
	require.Equal(t, "test", response.Body)
}

func TestSendNoRunner(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl, err := NewRunnerController(context.Background(), &RunnerControllerConfig{
		PubSub: ps,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err = ctrl.Send(ctx, "snowman", &types.Request{
		Method: "GET",
		URL:    "https://example.com",
		Body:   "{}",
	})
	require.Error(t, err)
}
