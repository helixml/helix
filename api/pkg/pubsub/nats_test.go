package pubsub

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNatsPubsub(t *testing.T) {

	t.Run("Subscribe", func(t *testing.T) {
		pubsub, err := NewInMemoryNats()
		require.NoError(t, err)

		ctx := context.Background()

		receivedCh := make(chan string, 1)

		consumer, err := pubsub.Subscribe(ctx, "test", func(payload []byte) error {
			receivedCh <- string(payload)
			return nil
		})
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		err = pubsub.Publish(ctx, "test", []byte("hello"))
		require.NoError(t, err)

		t.Log("published, waiting")

		result := <-receivedCh
		require.Equal(t, "hello", result)

		// Unsubscribe
		err = consumer.Unsubscribe()
		require.NoError(t, err)
	})

	t.Run("Subscribe_Wildcard", func(t *testing.T) {
		pubsub, err := NewInMemoryNats()
		require.NoError(t, err)

		ctx := context.Background()

		receivedCh := make(chan string, 1)

		consumer, err := pubsub.Subscribe(ctx, "test.*", func(payload []byte) error {
			receivedCh <- string(payload)
			return nil
		})
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		err = pubsub.Publish(ctx, "test.123", []byte("hello"))
		require.NoError(t, err)

		t.Log("published, waiting")

		result := <-receivedCh
		require.Equal(t, "hello", result)

		// Unsubscribe
		err = consumer.Unsubscribe()
		require.NoError(t, err)
	})

	t.Run("Subscribe_Resubscribe", func(t *testing.T) {
		pubsub, err := NewInMemoryNats()
		require.NoError(t, err)

		ctx := context.Background()

		receivedCh := make(chan string, 1)

		consumer, err := pubsub.Subscribe(ctx, "test", func(payload []byte) error {
			receivedCh <- string(payload)
			return nil
		})
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		err = pubsub.Publish(ctx, "test", []byte("hello"))
		require.NoError(t, err)

		t.Log("published, waiting")

		result := <-receivedCh
		require.Equal(t, "hello", result)

		// Unsubscribe
		err = consumer.Unsubscribe()
		require.NoError(t, err)

		// Subscribe again
		receivedCh2 := make(chan string, 1)
		consumer, err = pubsub.Subscribe(ctx, "test", func(payload []byte) error {
			receivedCh2 <- string(payload)
			return nil
		})
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		err = pubsub.Publish(ctx, "test", []byte("hello"))
		require.NoError(t, err)

		result = <-receivedCh2
		require.Equal(t, "hello", result)

		// Unsubscribe
		err = consumer.Unsubscribe()
		require.NoError(t, err)
	})
}
