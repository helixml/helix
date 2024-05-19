package pubsub

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
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

func TestQueueMultipleSubs(t *testing.T) {
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

	sub1, err := pubsub.QueueSubscribe(ctx, ScriptRunnerStream, AppQueue, 10, func(msg *Message) error {
		err := pubsub.Publish(ctx, msg.Reply, []byte("world"))
		require.NoError(t, err)
		msg.Ack()

		worker1++

		return nil
	})
	require.NoError(t, err)
	defer sub1.Unsubscribe()

	sub2, err := pubsub.QueueSubscribe(ctx, ScriptRunnerStream, AppQueue, 10, func(msg *Message) error {
		err := pubsub.Publish(ctx, msg.Reply, []byte("world"))
		require.NoError(t, err)
		msg.Ack()

		worker2++

		return nil
	})
	require.NoError(t, err)
	defer sub2.Unsubscribe()

	for i := 0; i < 100; i++ {
		data, err := pubsub.Request(ctx, ScriptRunnerStream, AppQueue, []byte(fmt.Sprintf("hello-%d", i)), map[string]string{}, 10*time.Second)
		require.NoError(t, err)

		require.Equal(t, "world", string(data))

		messageCounter++
	}

	assert.True(t, worker1 > 0)
	assert.True(t, worker2 > 0)

	assert.True(t, worker1 < 100)
	assert.True(t, worker2 < 100)

	assert.Equal(t, worker1+worker2, messageCounter, "should have the total 100 messages")

	t.Logf("worker1: %d", worker1)
	t.Logf("worker2: %d", worker2)
}

func TestNatsStreaming(t *testing.T) {
	t.Run("SubscribeLater", func(t *testing.T) {
		pubsub, err := NewInMemoryNats(t.TempDir())
		require.NoError(t, err)

		ctx := context.Background()

		messageCounter := 0

		go func() {
			for i := 0; i < 100; i++ {
				data, err := pubsub.StreamRequest(ctx, ScriptRunnerStream, AppQueue, []byte("hello"), map[string]string{
					"foo": "bar",
				}, 10*time.Second)
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

			assert.Equal(t, "bar", msg.Header.Get("foo"))

			return nil
		})
		require.NoError(t, err)
		defer sub.Unsubscribe()

		sub2, err := pubsub.StreamConsume(ctx, ScriptRunnerStream, AppQueue, 10, func(msg *Message) error {
			err := pubsub.Publish(ctx, msg.Reply, []byte("world"))
			require.NoError(t, err)

			assert.Equal(t, "bar", msg.Header.Get("foo"))

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
					t.Logf("waiting for messages %d/%d\n", messageCounter, 100)
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
		wg := conc.NewWaitGroup()

		for i := 0; i < 10; i++ {
			i := i
			wg.Go(func() {
				data, err := pubsub.StreamRequest(ctx, ScriptRunnerStream, AppQueue, []byte(fmt.Sprintf("data-%d", i)), map[string]string{}, 10*time.Second)
				require.NoError(t, err)

				require.Equal(t, "world", string(data))

				messageCounter.Add(1)
			})
		}

		wg.Wait()
	}()

	// Wait a bit before starting the work
	time.Sleep(1 * time.Second)

	var nacks int

	sub, err := pubsub.StreamConsume(ctx, ScriptRunnerStream, AppQueue, 10, func(msg *Message) error {
		t.Log("consume", string(msg.Data))
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
				t.Logf("waiting for messages %d/%d", val, 10)
			} else {
				return
			}
		}
	}
}

func TestStreamMultipleSubs(t *testing.T) {
	pubsub, err := NewInMemoryNats(t.TempDir())
	require.NoError(t, err)

	// Leaving a little bit of time to go into inactive state,
	// this was giving some flakiness in real deployment
	// vs tests
	time.Sleep(5 * time.Second)

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
		data, err := pubsub.StreamRequest(ctx, ScriptRunnerStream, AppQueue, []byte(fmt.Sprintf("hello-%d", i)), map[string]string{}, 10*time.Second)
		require.NoError(t, err)

		require.Equal(t, "world", string(data))

		messageCounter++
	}

	assert.True(t, worker1 > 0)
	assert.True(t, worker2 > 0)

	assert.True(t, worker1 < 100)
	assert.True(t, worker2 < 100)

	assert.Equal(t, worker1+worker2, messageCounter, "should have the total 100 messages")

	t.Logf("worker1: %d", worker1)
	t.Logf("worker2: %d", worker2)
}

func TestStreamAfterDelay(t *testing.T) {
	pubsub, err := NewInMemoryNats(t.TempDir())
	require.NoError(t, err)

	ctx := context.Background()

	var messageCounter atomic.Int32

	go func() {
		wg := conc.NewWaitGroup()

		for i := 0; i < 10; i++ {

			wg.Go(func() {
				data, err := pubsub.StreamRequest(ctx, ScriptRunnerStream, AppQueue, []byte(fmt.Sprintf("hello-%d", i)), map[string]string{}, 10*time.Second)
				require.NoError(t, err)

				require.Equal(t, "world", string(data))

				t.Log("received", string(data))

				messageCounter.Add(1)
			})
		}

		wg.Wait()
	}()

	// Wait a bit before starting the work
	time.Sleep(3 * time.Second)

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
			t.Logf("Message '%s', do nothing on %d\n", string(msg.Data), nacks)
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
				t.Logf("waiting for messages %d/%d\n", val, 10)
			} else {
				return
			}
		}
	}
}

func TestStreamFailOne(t *testing.T) {
	pubsub, err := NewInMemoryNats(t.TempDir())
	require.NoError(t, err)

	ctx := context.Background()

	var messageCounter atomic.Int32

	go func() {
		wg := conc.NewWaitGroup()

		for i := 0; i < 10; i++ {
			// Dispatch 10 requests
			wg.Go(func() {
				if i == 0 {
					// This one will fail on purpose
					_, err := pubsub.StreamRequest(ctx, ScriptRunnerStream, AppQueue, []byte(fmt.Sprintf("work-%d", i)), map[string]string{}, 10*time.Second)
					require.Error(t, err)
					return
				}
				// Others should succeed
				data, err := pubsub.StreamRequest(ctx, ScriptRunnerStream, AppQueue, []byte(fmt.Sprintf("work-%d", i)), map[string]string{}, 10*time.Second)
				require.NoError(t, err)

				require.Equal(t, "world", string(data))

				messageCounter.Add(1)
			})

			time.Sleep(50 * time.Millisecond)
		}

		wg.Wait()
	}()

	sub, err := pubsub.StreamConsume(ctx, ScriptRunnerStream, AppQueue, 10, func(msg *Message) error {
		if string(msg.Data) == "work-0" {
			// Don't ack or nack
			t.Log("will not process this message")
			return fmt.Errorf("failed to process")
		}

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

			if val == 10 {
				t.Fatalf("should not have received all messages")
			}

			if val < 9 {
				time.Sleep(500 * time.Millisecond)
				t.Logf("waiting for messages %d/%d\n", val, 9)
			} else {
				return
			}
		}
	}
}
