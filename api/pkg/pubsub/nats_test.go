package pubsub

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNatsPubsub(t *testing.T) {

	t.Run("Subscribe", func(t *testing.T) {
		pubsub, err := NewInMemoryNats(t.TempDir())
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
		pubsub, err := NewInMemoryNats(t.TempDir())
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
		pubsub, err := NewInMemoryNats(t.TempDir())
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

func TestNatsStreaming(t *testing.T) {
	t.Run("SubscribeLater", func(t *testing.T) {
		pubsub, err := NewInMemoryNats(t.TempDir())
		require.NoError(t, err)

		ctx := context.Background()

		receivedCh := make(chan []byte, 1)

		go func() {
			data, err := pubsub.Request(ctx, GetGPTScriptAppQueue(), []byte("hello"), 10*time.Second)
			require.NoError(t, err)

			receivedCh <- data
		}()

		// Wait a bit before starting the work
		time.Sleep(5 * time.Second)

		sub, err := pubsub.QueueSubscribe(ctx, GetGPTScriptAppQueue(), "worker", 10, func(reply string, payload []byte) error {
			fmt.Println("MSG RECEIVED, REPLYING TO", reply)

			err := pubsub.Publish(ctx, reply, []byte("world"))
			require.NoError(t, err)

			return nil
		})
		require.NoError(t, err)
		defer sub.Unsubscribe()

		result := <-receivedCh
		require.Equal(t, "world", string(result))
	})
}
