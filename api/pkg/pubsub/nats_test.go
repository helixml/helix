package pubsub

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
		time.Sleep(1 * time.Second)

		sub, err := pubsub.StreamConsume(ctx, ScriptRunnerStream, AppQueue, 10, func(msg *Message) error {
			err := pubsub.Publish(ctx, msg.Reply, []byte("world"))
			require.NoError(t, err)

			return nil
		})
		require.NoError(t, err)
		defer sub.Unsubscribe()

		sub2, err := pubsub.StreamConsume(ctx, ScriptRunnerStream, ToolQueue, 10, func(msg *Message) error {
			err := pubsub.Publish(ctx, msg.Reply, []byte("world"))
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

func TestStreamRetries(t *testing.T) {
	pubsub, err := NewInMemoryNats(t.TempDir())
	require.NoError(t, err)

	ctx := context.Background()

	var messageCounter atomic.Int32

	go func() {
		for i := 0; i < 10; i++ {
			data, err := pubsub.StreamRequest(ctx, ScriptRunnerStream, AppQueue, []byte("hello"), 10*time.Second)
			require.NoError(t, err)

			require.Equal(t, "world", string(data))

			messageCounter.Add(1)
		}
	}()

	// Wait a bit before starting the work
	time.Sleep(1 * time.Second)

	var nacks int

	sub, err := pubsub.StreamConsume(ctx, ScriptRunnerStream, AppQueue, 10, func(msg *Message) error {
		// Nack 5 messages and don't reply anything, they should be retried
		if nacks < 5 {
			nacks++
			t.Logf("Nack %d", nacks)
			return msg.Nak()
		}

		t.Logf("Ack %d", nacks)

		err := pubsub.Publish(ctx, msg.Reply, []byte("world"))
		require.NoError(t, err)

		msg.Ack()

		return nil
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			require.Fail(t, "timeout")
		default:
			val := messageCounter.Load()

			if val < 10 {
				time.Sleep(100 * time.Millisecond)
				fmt.Printf("waiting for messages %d/%d\n", val, 10)
			} else {
				return
			}
		}
	}
}

func TestStreamMultipleSubs(t *testing.T) {
	pubsub, err := NewInMemoryNats(t.TempDir())
	require.NoError(t, err)

	ctx := context.Background()

	messageCounter := 0

	// Wait a bit before starting the work
	time.Sleep(1 * time.Second)

	// Start two workers
	var (
		worker1 int
		worker2 int
	)

	sub1, err := pubsub.StreamConsume(ctx, ScriptRunnerStream, AppQueue, 10, func(msg *Message) error {
		err := pubsub.Publish(ctx, msg.Reply, []byte("world"))
		require.NoError(t, err)
		msg.Ack()

		worker1++

		return nil
	})
	require.NoError(t, err)
	defer sub1.Unsubscribe()

	sub2, err := pubsub.StreamConsume(ctx, ScriptRunnerStream, AppQueue, 10, func(msg *Message) error {
		err := pubsub.Publish(ctx, msg.Reply, []byte("world"))
		require.NoError(t, err)
		msg.Ack()

		worker2++

		return nil
	})
	require.NoError(t, err)
	defer sub2.Unsubscribe()

	for i := 0; i < 100; i++ {
		data, err := pubsub.StreamRequest(ctx, ScriptRunnerStream, AppQueue, []byte(fmt.Sprintf("hello-%d", i)), 10*time.Second)
		require.NoError(t, err)

		require.Equal(t, "world", string(data))

		messageCounter++
	}

	assert.True(t, worker1 > 0)
	assert.True(t, worker2 > 0)

	t.Logf("worker1: %d", worker1)
	t.Logf("worker2: %d", worker2)
}

func TestStreamAfterDelay(t *testing.T) {
	pubsub, err := NewInMemoryNats(t.TempDir())
	require.NoError(t, err)

	ctx := context.Background()

	messageCounter := 0

	go func() {
		for i := 0; i < 10; i++ {
			fmt.Println("publishing msg")
			data, err := pubsub.StreamRequest(ctx, ScriptRunnerStream, AppQueue, []byte(fmt.Sprintf("hello-%d", i)), 10*time.Second)
			require.NoError(t, err)

			require.Equal(t, "world", string(data))

			fmt.Println("received", string(data))

			messageCounter++
		}
	}()

	// Wait a bit before starting the work
	time.Sleep(1 * time.Second)

	var (
		nacks   int
		nacksMu sync.Mutex
	)

	sub, err := pubsub.StreamConsume(ctx, ScriptRunnerStream, AppQueue, 10, func(msg *Message) error {
		nacksMu.Lock()
		defer nacksMu.Unlock()

		// Ignore some messages
		if nacks < 5 {
			nacks++
			fmt.Printf("Message '%s', do nothing on %d\n", string(msg.Data), nacks)
			return msg.Nak()
		}

		t.Logf("Ack %d", nacks)

		err := pubsub.Publish(ctx, msg.Reply, []byte("world"))
		require.NoError(t, err)

		msg.Ack()

		return nil
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			require.Fail(t, "timeout")
		default:
			if messageCounter < 10 {
				time.Sleep(100 * time.Millisecond)
				fmt.Printf("waiting for messages %d/%d\n", messageCounter, 10)
			} else {
				return
			}
		}
	}
}
