package pubsub

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testWait = 5 * time.Second

func newTestNats(t *testing.T) *Nats {
	t.Helper()
	n, err := NewInMemoryNats()
	require.NoError(t, err)
	t.Cleanup(n.Close)
	return n
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(testWait):
		t.Fatal("timed out waiting for a message")
		var zero T
		return zero
	}
}

// subscribe registers a core subscription and waits for the server to have
// processed it, so a publish straight after cannot race the SUB.
func subscribe(t *testing.T, n *Nats, topic string) <-chan string {
	t.Helper()
	received := make(chan string, 1)
	sub, err := n.Subscribe(context.Background(), topic, func(payload []byte) error {
		received <- string(payload)
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	require.NoError(t, n.conn.Flush())
	return received
}

func TestSubscribe(t *testing.T) {
	cases := []struct {
		name, topic, publishTo string
	}{
		{name: "exact", topic: "test", publishTo: "test"},
		{name: "wildcard", topic: "test.*", publishTo: "test.123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := newTestNats(t)
			received := subscribe(t, n, tc.topic)

			require.NoError(t, n.Publish(context.Background(), tc.publishTo, []byte("hello")))
			assert.Equal(t, "hello", receive(t, received))
		})
	}
}

func TestSubscribeAfterUnsubscribe(t *testing.T) {
	n := newTestNats(t)
	ctx := context.Background()

	first := make(chan string, 1)
	sub, err := n.Subscribe(ctx, "test", func(payload []byte) error {
		first <- string(payload)
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, n.conn.Flush())
	require.NoError(t, n.Publish(ctx, "test", []byte("one")))
	assert.Equal(t, "one", receive(t, first))
	require.NoError(t, sub.Unsubscribe())

	second := subscribe(t, n, "test")
	require.NoError(t, n.Publish(ctx, "test", []byte("two")))
	assert.Equal(t, "two", receive(t, second))
	assert.Empty(t, first, "an unsubscribed handler must not receive")
}

// The embedded server enforces the token: a client without it is refused.
func TestEmbeddedServerRequiresToken(t *testing.T) {
	cfg := durableTestConfig(t.TempDir(), server.RANDOM_PORT, server.RANDOM_PORT)
	cfg.PubSub.Server.Token = "secret"
	n, err := NewNats(cfg)
	require.NoError(t, err)
	t.Cleanup(n.Close)

	_, err = nats.Connect(n.embeddedServer.ClientURL())
	require.ErrorIs(t, err, nats.ErrAuthorization)

	authed, err := nats.Connect(n.embeddedServer.ClientURL(), nats.Token("secret"))
	require.NoError(t, err)
	authed.Close()
}

func TestQueueMultipleSubs(t *testing.T) {
	n := newTestNats(t)
	ctx := context.Background()

	var workers [2]atomic.Int32
	for i := range workers {
		sub, err := n.QueueSubscribe(ctx, "workers", "jobs", func(msg *Message) error {
			// Count before replying: the requester returns on the reply.
			workers[i].Add(1)
			return n.Publish(ctx, msg.Reply, []byte("world"))
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })
	}
	require.NoError(t, n.conn.Flush())

	const requests = 100
	for i := range requests {
		data, err := n.QueueRequest(ctx, "workers", "jobs", fmt.Appendf(nil, "hello-%d", i), map[string]string{}, testWait)
		require.NoError(t, err)
		require.Equal(t, "world", string(data))
	}

	w1, w2 := workers[0].Load(), workers[1].Load()
	t.Logf("worker1: %d, worker2: %d", w1, w2)
	assert.Positive(t, w1)
	assert.Positive(t, w2)
	assert.EqualValues(t, requests, w1+w2, "each request is handled by exactly one worker")
}

// streamRequests sends one StreamRequest per payload concurrently and returns
// a channel with each request's outcome.
func streamRequests(n *Nats, timeout time.Duration, payloads ...string) <-chan error {
	results := make(chan error, len(payloads))
	for _, payload := range payloads {
		go func() {
			data, err := n.StreamRequest(context.Background(), ScriptRunnerStream, AppQueue, []byte(payload), map[string]string{"foo": "bar"}, timeout)
			if err == nil && string(data) != "world" {
				err = fmt.Errorf("%s: reply %q, want world", payload, data)
			}
			if err != nil {
				err = fmt.Errorf("%s: %w", payload, err)
			}
			results <- err
		}()
	}
	return results
}

func payloads(prefix string, count int) []string {
	out := make([]string, count)
	for i := range out {
		out[i] = fmt.Sprintf("%s-%d", prefix, i)
	}
	return out
}

// waitForStreamMessages blocks until the stream holds want unconsumed messages.
func waitForStreamMessages(t *testing.T, n *Nats, want uint64) {
	t.Helper()
	require.Eventually(t, func() bool {
		info, err := n.streams[ScriptRunnerStream].Info(context.Background())
		return err == nil && info.State.Msgs == want
	}, testWait, 10*time.Millisecond)
}

func reply(n *Nats, msg *Message) error {
	if err := n.Publish(context.Background(), msg.Reply, []byte("world")); err != nil {
		return err
	}
	return msg.Ack()
}

// Requests published before anyone consumes the stream are delivered once
// consumers subscribe, each to exactly one of them.
func TestStreamConsumeAfterPublish(t *testing.T) {
	n := newTestNats(t)
	ctx := context.Background()

	const requests = 20
	results := streamRequests(n, testWait, payloads("hello", requests)...)
	waitForStreamMessages(t, n, requests)

	var handled atomic.Int32
	var badHeader atomic.Value
	for range 2 {
		sub, err := n.StreamConsume(ctx, ScriptRunnerStream, AppQueue, func(msg *Message) error {
			if got := msg.Header.Get("foo"); got != "bar" {
				badHeader.Store(got)
			}
			handled.Add(1)
			return reply(n, msg)
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })
	}

	for range requests {
		require.NoError(t, receive(t, results))
	}
	assert.Nil(t, badHeader.Load(), "headers must reach the consumer")
	assert.EqualValues(t, requests, handled.Load())
}

// A NAKed message is redelivered until a consumer replies to it.
func TestStreamConsumeRedeliversNak(t *testing.T) {
	n := newTestNats(t)

	const requests, naks = 10, 5
	results := streamRequests(n, testWait, payloads("data", requests)...)
	waitForStreamMessages(t, n, requests)

	var nacked atomic.Int32
	sub, err := n.StreamConsume(context.Background(), ScriptRunnerStream, AppQueue, func(msg *Message) error {
		if nacked.Add(1) <= naks {
			return msg.Nak()
		}
		return reply(n, msg)
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	for range requests {
		require.NoError(t, receive(t, results))
	}
	assert.EqualValues(t, requests+naks, nacked.Load(), "every NAK costs exactly one redelivery")
}

func TestStreamMultipleSubs(t *testing.T) {
	n := newTestNats(t)
	ctx := context.Background()

	var workers [2]atomic.Int32
	for i := range workers {
		sub, err := n.StreamConsume(ctx, ScriptRunnerStream, AppQueue, func(msg *Message) error {
			workers[i].Add(1)
			return reply(n, msg)
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = sub.Unsubscribe() })
	}

	const requests = 100
	for i := range requests {
		data, err := n.StreamRequest(ctx, ScriptRunnerStream, AppQueue, fmt.Appendf(nil, "hello-%d", i), map[string]string{}, testWait)
		require.NoError(t, err)
		require.Equal(t, "world", string(data))
	}

	w1, w2 := workers[0].Load(), workers[1].Load()
	t.Logf("worker1: %d, worker2: %d", w1, w2)
	assert.Positive(t, w1)
	assert.Positive(t, w2)
	assert.EqualValues(t, requests, w1+w2, "each request is handled by exactly one worker")
}

// A message the consumer never acks or answers fails its request at the
// timeout, without holding up the others.
func TestStreamFailOne(t *testing.T) {
	n := newTestNats(t)

	sub, err := n.StreamConsume(context.Background(), ScriptRunnerStream, AppQueue, func(msg *Message) error {
		if string(msg.Data) == "work-0" {
			return fmt.Errorf("failed to process")
		}
		return reply(n, msg)
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	failed := streamRequests(n, 500*time.Millisecond, "work-0")
	succeeded := streamRequests(n, testWait, payloads("work", 10)[1:]...)

	for range 9 {
		require.NoError(t, receive(t, succeeded))
	}
	require.ErrorContains(t, receive(t, failed), "timed out")
}

// Unsubscribe stops delivery even though the subscription's context is live.
func TestStreamConsumeUnsubscribeStopsDelivery(t *testing.T) {
	n := newTestNats(t)

	var handled atomic.Int32
	sub, err := n.StreamConsume(context.Background(), ScriptRunnerStream, AppQueue, func(msg *Message) error {
		handled.Add(1)
		return reply(n, msg)
	})
	require.NoError(t, err)
	require.NoError(t, receive(t, streamRequests(n, testWait, "before")))
	require.NoError(t, sub.Unsubscribe())

	results := streamRequests(n, 300*time.Millisecond, "after")
	require.ErrorContains(t, receive(t, results), "timed out")
	assert.EqualValues(t, 1, handled.Load())
}
