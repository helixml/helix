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

		messageCounter := 0

		go func() {
			for i := 0; i < 100; i++ {
				data, err := pubsub.StreamRequest(ctx, ScriptRunnerStream, AppQueue, []byte("hello"), 10*time.Second)
				require.NoError(t, err)

				require.Equal(t, "world", string(data))

				messageCounter++
			}
		}()

		// Wait a bit before starting the work
		time.Sleep(2 * time.Second)

		sub, err := pubsub.StreamConsume(ctx, ScriptRunnerStream, AppQueue, 10, func(reply string, payload []byte) error {
			err := pubsub.Publish(ctx, reply, []byte("world"))
			require.NoError(t, err)

			return nil
		})
		require.NoError(t, err)
		defer sub.Unsubscribe()

		sub2, err := pubsub.StreamConsume(ctx, ScriptRunnerStream, ToolQueue, 10, func(reply string, payload []byte) error {
			err := pubsub.Publish(ctx, reply, []byte("world"))
			require.NoError(t, err)

			return nil
		})
		require.NoError(t, err)
		defer sub2.Unsubscribe()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				require.Fail(t, "timeout")
			default:
				if messageCounter < 100 {
					time.Sleep(100 * time.Millisecond)
					fmt.Printf("waiting for messages %d/%d\n", messageCounter, 100)
				} else {
					return
				}
			}
		}
	})
}
