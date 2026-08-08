package pubsub

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

func TestDurableConsumerRedeliversNak(t *testing.T) {
	n, cleanup := setupTestNats(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, n.EnsurePersistentStream(ctx, "DURABLE_REDELIVERY", []string{"durable.redelivery.*"}))
	deliveries := make(chan []byte, 2)
	attempts := 0
	sub, err := n.ConsumeDurable(ctx, "DURABLE_REDELIVERY", "worker-one", "durable.redelivery.one", time.Second, func(msg *Message) error {
		attempts++
		deliveries <- msg.Data
		if attempts == 1 {
			return msg.Nak()
		}
		return msg.Ack()
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()
	require.NoError(t, n.PublishDurable(ctx, "DURABLE_REDELIVERY", "durable.redelivery.one", []byte("hello")))

	require.Equal(t, []byte("hello"), <-deliveries)
	require.Equal(t, []byte("hello"), <-deliveries)

	stream, err := n.js.Stream(ctx, "DURABLE_REDELIVERY")
	require.NoError(t, err)
	streamInfo, err := stream.Info(ctx)
	require.NoError(t, err)
	require.Equal(t, jetstream.FileStorage, streamInfo.Config.Storage)
	require.Zero(t, streamInfo.Config.MaxAge)
	consumer, err := stream.Consumer(ctx, "worker-one")
	require.NoError(t, err)
	consumerInfo, err := consumer.Info(ctx)
	require.NoError(t, err)
	require.Equal(t, jetstream.AckExplicitPolicy, consumerInfo.Config.AckPolicy)
	require.Equal(t, "durable.redelivery.one", consumerInfo.Config.FilterSubject)
	require.Equal(t, 1, consumerInfo.Config.MaxAckPending)
	require.Zero(t, consumerInfo.Config.InactiveThreshold)
}

func TestDurableConsumerSurvivesServerRestart(t *testing.T) {
	storeDir := t.TempDir()
	serverPort, websocketPort, err := getRandomPorts()
	require.NoError(t, err)
	cfg := durableTestConfig(storeDir, serverPort, websocketPort)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	first, err := NewNats(cfg)
	require.NoError(t, err)
	require.NoError(t, first.EnsurePersistentStream(ctx, "DURABLE_RESTART", []string{"durable.restart.*"}))
	sub, err := first.ConsumeDurable(ctx, "DURABLE_RESTART", "worker-one", "durable.restart.one", time.Second, func(*Message) error { return nil })
	require.NoError(t, err)
	require.NoError(t, sub.Unsubscribe())
	require.NoError(t, first.PublishDurable(ctx, "DURABLE_RESTART", "durable.restart.one", []byte("persisted")))
	stopTestNats(first)

	second, err := NewNats(cfg)
	require.NoError(t, err)
	defer stopTestNats(second)
	consumers, err := second.ListDurableConsumers(ctx, "DURABLE_RESTART")
	require.NoError(t, err)
	require.Equal(t, []DurableConsumer{{Name: "worker-one", Subject: "durable.restart.one"}}, consumers)
	received := make(chan []byte, 1)
	sub, err = second.ConsumeDurable(ctx, "DURABLE_RESTART", "worker-one", "durable.restart.one", time.Second, func(msg *Message) error {
		received <- msg.Data
		return msg.Ack()
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()
	require.Equal(t, []byte("persisted"), <-received)
}

func durableTestConfig(storeDir string, serverPort, websocketPort int) *config.ServerConfig {
	cfg := &config.ServerConfig{}
	cfg.PubSub.StoreDir = storeDir
	cfg.PubSub.Server.Host = "127.0.0.1"
	cfg.PubSub.Server.Port = serverPort
	cfg.PubSub.Server.WebsocketPort = websocketPort
	cfg.PubSub.Server.JetStream = true
	cfg.PubSub.Server.MaxPayload = 32 * 1024 * 1024
	cfg.PubSub.Server.EmbeddedNatsServerEnabled = true
	return cfg
}

func stopTestNats(n *Nats) {
	n.conn.Close()
	n.embeddedServer.Shutdown()
	n.embeddedServer.WaitForShutdown()
}
