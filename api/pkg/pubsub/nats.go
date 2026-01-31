package pubsub

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/freeport"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog/log"
)

const (
	zedAgentStreamName     = "ZED_AGENTS_STREAM"
	zedAgentSubject        = "ZED_AGENTS.*"
	helixNatsReplyHeader   = "helix-reply"
	helixNatsSubjectHeader = "helix-subject"
)

// ConnectionStatus represents the current connection state
type ConnectionStatus string

const (
	Connected    ConnectionStatus = "connected"
	Disconnected ConnectionStatus = "disconnected"
	Reconnecting ConnectionStatus = "reconnecting"
)

type ConnectionStatusHandler func(status ConnectionStatus)

type Nats struct {
	conn           *nats.Conn
	js             jetstream.JetStream
	embeddedServer *server.Server

	stream jetstream.Stream

	consumerMu sync.Mutex
	consumer   jetstream.Consumer

	// Zed agent stream and consumer (separate from GPTScript)
	zedStream   jetstream.Stream
	zedConsumer jetstream.Consumer

	// Support for multiple streams
	streams   map[string]jetstream.Stream
	consumers map[string]jetstream.Consumer
	streamMu  sync.RWMutex

	statusHandlers []ConnectionStatusHandler
	statusMu       sync.RWMutex
}

func (n *Nats) OnConnectionStatus(handler ConnectionStatusHandler) {
	n.statusMu.Lock()
	defer n.statusMu.Unlock()
	n.statusHandlers = append(n.statusHandlers, handler)
}

func (n *Nats) notifyStatusChange(status ConnectionStatus) {
	n.statusMu.RLock()
	defer n.statusMu.RUnlock()
	for _, handler := range n.statusHandlers {
		handler(status)
	}
}

func setupConnectionHandlers(nc *nats.Conn, n *Nats) {
	nc.SetDisconnectErrHandler(func(_ *nats.Conn, err error) {
		log.Warn().Err(err).Msg("nats connection lost")
		n.notifyStatusChange(Disconnected)
	})

	nc.SetReconnectHandler(func(_ *nats.Conn) {
		log.Info().Msg("nats reconnecting")
		n.notifyStatusChange(Reconnecting)
	})

	nc.SetClosedHandler(func(_ *nats.Conn) {
		log.Warn().Msg("nats connection closed")
		n.notifyStatusChange(Disconnected)
	})

	nc.SetDiscoveredServersHandler(func(_ *nats.Conn) {
		log.Debug().Strs("servers", nc.DiscoveredServers()).Msg("discovered nats servers")
	})

	// Use reconnect handler for connected state since there's no specific connected handler
	nc.SetReconnectHandler(func(_ *nats.Conn) {
		log.Info().Msg("nats connected")
		n.notifyStatusChange(Connected)
	})
}

// getRandomPorts returns a tuple of random available ports for server and websocket
func getRandomPorts() (int, int, error) {
	serverPort, err := freeport.GetFreePort()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get free server port: %w", err)
	}

	wsPort, err := freeport.GetFreePort()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get free websocket port: %w", err)
	}

	return serverPort, wsPort, nil
}

// tryStartServer attempts to start the NATS server with given ports
// returns the server instance and any error that occurred
func tryStartServer(cfg *config.ServerConfig, serverPort, wsPort int) (*server.Server, error) {
	opts := &server.Options{
		Debug:         true,
		Trace:         true,
		Host:          "127.0.0.1", // For internal use only
		Port:          serverPort,
		JetStream:     cfg.PubSub.Server.JetStream,
		StoreDir:      cfg.PubSub.StoreDir,
		MaxPayload:    int32(cfg.PubSub.Server.MaxPayload),
		Authorization: cfg.PubSub.Server.Token,
		AllowNonTLS:   true, // TLS is terminated at the reverse proxy
		Websocket: server.WebsocketOpts{
			Host:  cfg.PubSub.Server.Host,
			Port:  wsPort,
			NoTLS: true,
			Token: cfg.PubSub.Server.Token,
		},
	}

	// Initialize new server with options
	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create nats server: %w", err)
	}

	// Start the server via goroutine
	go ns.Start()

	// Wait for server to be ready for connections
	if !ns.ReadyForConnections(4 * time.Second) {
		ns.Shutdown()
		return nil, fmt.Errorf("server failed to start (ports %d, %d): running=%v", serverPort, wsPort, ns.Running())
	}

	log.Info().
		Str("internal_url", ns.ClientURL()).
		Str("external_url", fmt.Sprintf("ws://%s:%d", cfg.PubSub.Server.Host, wsPort)).
		Str("store_dir", cfg.PubSub.StoreDir).
		Msg("nats server started successfully")

	return ns, nil
}

// NewNats creates a new NATS instance with the given configuration
func NewNats(cfg *config.ServerConfig) (*Nats, error) {
	var ns *server.Server
	var err error

	// Falback to runner token if no token is provided
	if cfg.PubSub.Server.Token == "" {
		cfg.PubSub.Server.Token = cfg.WebServer.RunnerToken
	}

	// Create and start embedded server if we're not connecting to an external one
	if cfg.PubSub.Server.EmbeddedNatsServerEnabled {
		// Check store directory permissions
		if err := checkStoreDir(cfg.PubSub.StoreDir); err != nil {
			return nil, fmt.Errorf("nats store directory issue: %w", err)
		}

		maxRetries := 5
		var lastErr error

		for i := 0; i < maxRetries; i++ {
			// Always get new random ports on retry
			serverPort, wsPort, err := getRandomPorts()
			if err != nil {
				lastErr = err
				continue
			}

			// If ports were specified in config, try those first
			if i == 0 && cfg.PubSub.Server.Port != 0 && cfg.PubSub.Server.WebsocketPort != 0 {
				serverPort = cfg.PubSub.Server.Port
				wsPort = cfg.PubSub.Server.WebsocketPort
			}

			ns, err = tryStartServer(cfg, serverPort, wsPort)
			if err != nil {
				lastErr = err
				log.Debug().Err(err).Int("attempt", i+1).Msg("retrying nats server start with different ports")
				continue
			}

			// Server started successfully
			break
		}

		// If we exhausted all retries, return the last error
		if ns == nil {
			return nil, fmt.Errorf("failed to start nats server after %d retries: %w", maxRetries, lastErr)
		}
	}

	// Connect to server
	var nc *nats.Conn
	if ns != nil {
		// Connect to embedded server
		opts := []nats.Option{}
		if cfg.PubSub.Server.Token != "" {
			opts = append(opts, nats.Token(cfg.PubSub.Server.Token))
		}
		log.Info().Str("url", ns.ClientURL()).Msg("connecting to embedded nats")
		nc, err = nats.Connect(ns.ClientURL(), opts...)
	} else {
		// Connect to external server
		serverURL := fmt.Sprintf("nats://%s:%d", cfg.PubSub.Server.Host, cfg.PubSub.Server.Port)
		opts := []nats.Option{}
		if cfg.PubSub.Server.Token != "" {
			opts = append(opts, nats.Token(cfg.PubSub.Server.Token))
		}
		log.Info().Str("url", serverURL).Msg("connecting to external nats")
		nc, err = nats.Connect(serverURL, opts...)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to connect to nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create jetstream: %w", err)
	}

	// Initialize Nats struct early so we can use n variable
	n := &Nats{
		conn:           nc,
		embeddedServer: ns,
		js:             js,
		streams:        make(map[string]jetstream.Stream),
		consumers:      make(map[string]jetstream.Consumer),
		statusHandlers: make([]ConnectionStatusHandler, 0),
	}

	// Clean up old streams
	gcJetStream(js)

	// Create ZED_AGENTS stream for external agent runners
	stream, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:      zedAgentStreamName,
		Subjects:  []string{zedAgentSubject},
		Retention: jetstream.WorkQueuePolicy,
		Discard:   jetstream.DiscardOld,
		MaxAge:    5 * time.Minute, // Discard messages older than 5 minutes
	})
	if err != nil {
		if n.embeddedServer != nil {
			n.embeddedServer.Shutdown()
		}
		return nil, fmt.Errorf("failed to create zed agent stream: %w", err)
	}

	// Create SCRIPT_RUNNERS stream for script execution (used by tests and GPTScript integration)
	scriptStream, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:      "SCRIPT_RUNNERS",
		Subjects:  []string{"SCRIPT_RUNNERS.*"},
		Retention: jetstream.WorkQueuePolicy,
		Discard:   jetstream.DiscardOld,
		MaxAge:    5 * time.Minute,
	})
	if err != nil {
		if n.embeddedServer != nil {
			n.embeddedServer.Shutdown()
		}
		return nil, fmt.Errorf("failed to create script runner stream: %w", err)
	}

	// Store streams in map
	n.stream = stream
	n.streams["ZED_AGENTS"] = stream
	n.streams["SCRIPT_RUNNERS"] = scriptStream

	ctx := context.Background()
	consumerName := "helix-zed-agent-consumer"

	log.Info().
		Str("stream", zedAgentStreamName).
		Str("consumer", consumerName).
		Msg("initializing ZED_AGENTS consumer")

	// First try to get existing consumer with the same name
	var c jetstream.Consumer
	if existingConsumer, err := stream.Consumer(ctx, consumerName); err == nil {
		// Consumer already exists, check if configuration matches
		info, err := existingConsumer.Info(ctx)
		if err == nil {
			expectedFilterSubject := getStreamSub(ZedAgentRunnerStream, ZedAgentQueue)
			if len(info.Config.FilterSubjects) == 1 && info.Config.FilterSubjects[0] == expectedFilterSubject {
				log.Info().
					Str("consumer", consumerName).
					Str("filter_subject", expectedFilterSubject).
					Msg("reusing existing consumer with matching configuration")
				c = existingConsumer
			} else {
				log.Warn().
					Str("consumer", consumerName).
					Strs("existing_filters", info.Config.FilterSubjects).
					Str("expected_filter", expectedFilterSubject).
					Msg("existing consumer has different configuration, deleting and recreating")
				if err := js.DeleteConsumer(ctx, zedAgentStreamName, consumerName); err != nil {
					log.Err(err).Str("consumer", consumerName).Msg("failed to delete existing consumer")
				}
				c = nil // Force recreation
			}
		} else {
			log.Warn().Err(err).Str("consumer", consumerName).Msg("failed to get existing consumer info, will try to recreate")
			c = nil // Force recreation
		}
	} else {
		log.Info().Str("consumer", consumerName).Msg("consumer doesn't exist, creating new one")
	}

	// Create consumer if we don't have one yet
	if c == nil {
		var err error
		c, err = stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			Name:           consumerName,
			AckPolicy:      jetstream.AckExplicitPolicy,
			FilterSubjects: []string{getStreamSub(ZedAgentRunnerStream, ZedAgentQueue)},
			AckWait:        5 * time.Second,
			BackOff:        []time.Duration{1 * time.Second, 5 * time.Second, 30 * time.Second}, // Exponential backoff to prevent log spam
			ReplayPolicy:   jetstream.ReplayInstantPolicy,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create consumer: %w", err)
		}

		log.Info().
			Str("consumer", consumerName).
			Str("stream", zedAgentStreamName).
			Str("filter_subject", getStreamSub(ZedAgentRunnerStream, ZedAgentQueue)).
			Msg("successfully created ZED_AGENTS consumer")
	}
	n.consumer = c
	n.consumers[consumerName] = c

	// Basic monitoring of the stream
	go func() {
		for {
			info, err := stream.Info(ctx)
			if err != nil {
				log.Err(err).Msg("failed to get stream info")
				time.Sleep(1 * time.Second) // Wait before retrying on error
				continue
			}
			if info.State.Consumers == 0 {
				log.Trace().
					Int("messages", int(info.State.Msgs)).
					Int("consumers", info.State.Consumers).
					Time("oldest_message", info.State.FirstTime).
					Time("newest_message", info.State.LastTime).
					Msg("Stream info")
			} else if info.State.Msgs > 100 {
				log.Debug().
					Int("messages", int(info.State.Msgs)).
					Int("consumers", info.State.Consumers).
					Msg("ZED_AGENTS stream has many pending messages")
			}

			time.Sleep(10 * time.Second)
		}
	}()

	// Update the Nats struct with stream and consumer
	n.stream = stream
	n.consumer = c

	// Setup connection monitoring
	setupConnectionHandlers(nc, n)

	// Initial connection status
	n.notifyStatusChange(Connected)

	return n, nil
}

// NewInMemoryNats creates a new in-memory NATS instance for testing
func NewInMemoryNats() (*Nats, error) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "helix-nats")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	randomPort, err := freeport.GetFreePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get free port: %w", err)
	}

	cfg := &config.ServerConfig{}
	cfg.PubSub.StoreDir = tmpDir
	cfg.PubSub.Server.Host = "0.0.0.0"
	cfg.PubSub.Server.Port = randomPort
	cfg.PubSub.Server.WebsocketPort = randomPort + 1
	cfg.PubSub.Server.JetStream = true
	cfg.PubSub.Server.MaxPayload = 32 * 1024 * 1024 // 32MB
	cfg.PubSub.Server.EmbeddedNatsServerEnabled = true

	return NewNats(cfg)
}

func NewNatsClient(u string, token string) (*Nats, error) {
	// Parse the URL to get the host and path
	parsedURL, err := url.Parse(u)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	opts := []nats.Option{
		nats.Token(token),
		nats.Timeout(time.Second * 5),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second * 2),
		nats.ReconnectJitter(time.Second, time.Second*5),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Warn().Err(err).Msg("disconnected from NATS server")
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Info().Str("url", nc.ConnectedUrl()).Msg("reconnected to NATS server")
		}),
		nats.ErrorHandler(func(_ *nats.Conn, sub *nats.Subscription, err error) {
			log.Error().Err(err).Str("subject", sub.Subject).Msg("NATS error")
		}),
		nats.ProxyPath(parsedURL.Path),
	}

	hostURL := parsedURL.Scheme + "://" + parsedURL.Host
	log.Info().Str("host", hostURL).Str("proxy_path", parsedURL.Path).Msg("connecting to nats")
	nc, err := nats.Connect(hostURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create jetstream context: %w", err)
	}

	n := &Nats{
		conn:           nc,
		js:             js,
		statusHandlers: make([]ConnectionStatusHandler, 0),
	}

	// Setup connection monitoring
	setupConnectionHandlers(nc, n)

	// Initial connection status
	n.notifyStatusChange(Connected)

	return n, nil
}

func (n *Nats) Subscribe(_ context.Context, topic string, handler func(payload []byte) error) (Subscription, error) {
	sub, err := n.conn.Subscribe(topic, func(msg *nats.Msg) {
		err := handler(msg.Data)
		if err != nil {
			log.Err(err).Msg("error handling message")
		}
	})
	if err != nil {
		return nil, err
	}

	return sub, nil
}

func (n *Nats) SubscribeWithCtx(_ context.Context, topic string, handler func(ctx context.Context, msg *nats.Msg) error) (Subscription, error) {
	sub, err := n.conn.Subscribe(topic, func(msg *nats.Msg) {
		err := handler(context.Background(), msg)
		if err != nil {
			log.Err(err).Msg("error handling message")
		}
	})
	if err != nil {
		return nil, err
	}

	return sub, nil
}

func (n *Nats) Publish(_ context.Context, topic string, payload []byte) error {
	return n.conn.Publish(topic, payload)
}

func (n *Nats) PublishWithHeader(_ context.Context, topic string, header map[string]string, payload []byte) error {
	hdr := nats.Header{}

	for k, v := range header {
		hdr.Set(k, v)
	}

	return n.conn.PublishMsg(&nats.Msg{
		Subject: topic,
		Data:    payload,
		Header:  hdr,
	})
}

func (n *Nats) Request(ctx context.Context, sub string, header map[string]string, payload []byte, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	hdr := nats.Header{}

	for k, v := range header {
		hdr.Set(k, v)
	}

	msg := &nats.Msg{
		Subject: sub,
		Data:    payload,
		Header:  hdr,
	}

	msg, err := n.conn.RequestMsgWithContext(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to request message: %w", err)
	}

	return msg.Data, nil
}

func (n *Nats) QueueRequest(ctx context.Context, _, subject string, payload []byte, header map[string]string, timeout time.Duration) ([]byte, error) {
	replyInbox := nats.NewInbox()
	var dataCh = make(chan []byte)

	sub, err := n.conn.Subscribe(replyInbox, func(msg *nats.Msg) {
		dataCh <- msg.Data
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := sub.Unsubscribe(); err != nil {
			log.Error().Err(err).Msg("failed to unsubscribe")
		}
	}()

	hdr := nats.Header{}

	for k, v := range header {
		hdr.Set(k, v)
	}

	hdr.Set(helixNatsReplyHeader, replyInbox)
	hdr.Set(helixNatsSubjectHeader, subject)

	// Publish the message to NATS
	err = n.conn.PublishMsg(&nats.Msg{
		Subject: subject,
		Data:    payload,
		Header:  hdr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to publish message to jetstream: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(timeout):
		return nil, fmt.Errorf("request timed out after %v", timeout)
	case data := <-dataCh:
		return data, nil
	}
}

func (n *Nats) QueueSubscribe(_ context.Context, queue, subject string, handler func(msg *Message) error) (Subscription, error) {
	sub, err := n.conn.QueueSubscribe(subject, queue, func(msg *nats.Msg) {
		err := handler(&Message{
			Reply:  msg.Header.Get(helixNatsReplyHeader),
			Data:   msg.Data,
			Type:   msg.Header.Get(helixNatsSubjectHeader),
			Header: msg.Header,
			msg:    &natsMsgWrapper{msg},
		})
		if err != nil {
			log.Err(err).Msg("error handling message")
		}
	})
	if err != nil {
		return nil, err
	}

	return sub, nil
}

// Request publish a message to the given subject and creates an inbox to receive the response. If response is not
// received within the timeout, an error is returned.
func (n *Nats) StreamRequest(ctx context.Context, stream, subject string, payload []byte, header map[string]string, timeout time.Duration) ([]byte, error) {
	replyInbox := nats.NewInbox()
	var dataCh = make(chan []byte)

	sub, err := n.conn.Subscribe(replyInbox, func(msg *nats.Msg) {
		dataCh <- msg.Data
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := sub.Unsubscribe(); err != nil {
			log.Error().Err(err).Msg("failed to unsubscribe")
		}
	}()

	hdr := nats.Header{}

	for k, v := range header {
		hdr.Set(k, v)
	}

	hdr.Set(helixNatsReplyHeader, replyInbox)
	hdr.Set(helixNatsSubjectHeader, subject)

	// streamTopic := getStreamSub(stream, subject) + "." + nuid.Next()
	streamTopic := getStreamSub(stream, subject)

	log.Debug().
		Str("EXTERNAL_AGENT_DEBUG", "publishing_to_jetstream").
		Str("stream_topic", streamTopic).
		Str("stream", stream).
		Str("subject", subject).
		Msg("🚀 EXTERNAL_AGENT_DEBUG: Publishing message to JetStream")

	// Publish the message to the JetStream stream,
	// one of the consumer will pick it up
	ackFuture, err := n.js.PublishMsg(ctx, &nats.Msg{
		Subject: streamTopic,
		Data:    payload,
		Header:  hdr,
	},
		jetstream.WithRetryWait(50*time.Millisecond),
		jetstream.WithRetryAttempts(10),
	)
	if err != nil {
		log.Error().
			Str("EXTERNAL_AGENT_DEBUG", "jetstream_publish_error").
			Err(err).
			Str("stream_topic", streamTopic).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to publish to JetStream")
		return nil, fmt.Errorf("failed to publish message to jetstream: %w", err)
	}

	// Log the JetStream acknowledgment info
	if ackFuture != nil {
		log.Debug().
			Str("EXTERNAL_AGENT_DEBUG", "jetstream_publish_ack").
			Str("stream_topic", streamTopic).
			Str("stream_name", ackFuture.Stream).
			Uint64("sequence", ackFuture.Sequence).
			Bool("duplicate", ackFuture.Duplicate).
			Msg("✅ EXTERNAL_AGENT_DEBUG: JetStream acknowledged message")
	} else {
		log.Warn().
			Str("EXTERNAL_AGENT_DEBUG", "jetstream_publish_no_ack").
			Str("stream_topic", streamTopic).
			Msg("⚠️ EXTERNAL_AGENT_DEBUG: JetStream publish returned no acknowledgment")
	}

	select {
	case <-ctx.Done():
		info, err := n.stream.Info(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to get stream info: %w", err)
		}

		if info.State.Consumers == 0 {
			return nil, fmt.Errorf("no consumers available to process the request, are there any runner connected?")
		}

		return nil, ctx.Err()
	case <-time.After(timeout):
		return nil, fmt.Errorf("request timed out after %v", timeout)
	case data := <-dataCh:
		return data, nil
	}
}

// QueueSubscribe is similar to Subscribe, but it will only deliver a message to one subscriber in the group. This way you can
// have multiple subscribers to the same subject, but only one gets it.
func (n *Nats) StreamConsume(ctx context.Context, stream, subject string, handler func(msg *Message) error) (Subscription, error) {
	log.Info().
		Str("ZED_FLOW_DEBUG", "stream_consume_simple_pattern").
		Str("stream", stream).
		Str("subject", subject).
		Msg("🎯 ZED_FLOW_DEBUG: Using SIMPLE GPTScript pattern - single anonymous consumer")

	n.consumerMu.Lock()
	defer n.consumerMu.Unlock()

	// Use the EXACT same simple pattern as working GPTScript implementation
	// Get the appropriate stream (ZED_AGENTS for Zed, SCRIPTS for GPTScript)
	var targetStream jetstream.Stream
	var err error

	// Get the appropriate stream from the streams map
	if storedStream, ok := n.streams[stream]; ok {
		targetStream = storedStream
	} else {
		return nil, fmt.Errorf("stream %s not found", stream)
	}

	// Check existing consumers - EXACT same logic as GPTScript
	info, err := targetStream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	log.Info().
		Str("ZED_FLOW_DEBUG", "checking_existing_consumers").
		Str("stream", stream).
		Int("consumer_count", info.State.Consumers).
		Msg("🔧 ZED_FLOW_DEBUG: Checking if consumer already exists")

	// EXACT same logic as GPTScript - only create if no consumers exist
	if info.State.Consumers == 0 {
		log.Info().
			Str("ZED_FLOW_DEBUG", "creating_anonymous_consumer").
			Str("stream", stream).
			Msg("🆕 ZED_FLOW_DEBUG: No consumers exist - creating anonymous consumer like GPTScript")

		// Create ANONYMOUS consumer (no Name field) - EXACT same as GPTScript
		c, err := targetStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			// ✅ NO NAME FIELD - This prevents the "not unique" error
			AckPolicy:      jetstream.AckExplicitPolicy,
			FilterSubjects: []string{getStreamSub(stream, subject)},
			AckWait:        5 * time.Second,
			BackOff:        []time.Duration{1 * time.Second, 5 * time.Second, 30 * time.Second},
			ReplayPolicy:   jetstream.ReplayInstantPolicy,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create consumer: %w", err)
		}

		// Store consumer based on stream type (like GPTScript stores in n.consumer)
		if stream == ZedAgentRunnerStream {
			n.zedConsumer = c
		} else {
			n.consumer = c
		}

		log.Info().
			Str("ZED_FLOW_DEBUG", "anonymous_consumer_created").
			Str("stream", stream).
			Msg("✅ ZED_FLOW_DEBUG: Anonymous consumer created successfully")
	}

	// EXACT GPTScript pattern - just use the stored consumer, no recreation logic
	var activeConsumer jetstream.Consumer
	if stream == ZedAgentRunnerStream {
		activeConsumer = n.zedConsumer
	} else {
		activeConsumer = n.consumer
	}

	// If consumer is nil, it means we need to use the startup-created consumer
	// The startup code creates helix-zed-agent-consumer, but we need to get a reference to it
	if activeConsumer == nil && info.State.Consumers > 0 {
		log.Info().
			Str("ZED_FLOW_DEBUG", "using_startup_consumer").
			Str("stream", stream).
			Int("consumer_count", info.State.Consumers).
			Msg("🔧 ZED_FLOW_DEBUG: No local consumer reference - using startup-created consumer")

		// Get the existing consumer created at startup (don't create a new one!)
		consumers := targetStream.ListConsumers(ctx)

		// Iterate over the channel to get the first consumer
		for consumerInfo := range consumers.Info() {
			log.Info().
				Str("ZED_FLOW_DEBUG", "found_existing_consumer").
				Str("consumer_name", consumerInfo.Name).
				Msg("📍 ZED_FLOW_DEBUG: Found existing consumer from startup")

			// Get consumer by name
			consumer, err := targetStream.Consumer(ctx, consumerInfo.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to get consumer %s: %w", consumerInfo.Name, err)
			}

			// Store the consumer reference
			if stream == ZedAgentRunnerStream {
				n.zedConsumer = consumer
				activeConsumer = consumer
			} else {
				n.consumer = consumer
				activeConsumer = consumer
			}
			break // Use the first consumer
		}
	}

	mc, err := activeConsumer.Messages()
	if err != nil {
		return nil, fmt.Errorf("failed to get messages context: %w", err)
	}

	go func() {
		<-ctx.Done()
		mc.Stop()
	}()

	go func() {
		log.Info().
			Str("ZED_FLOW_DEBUG", "nats_consumer_loop_start").
			Str("stream", stream).
			Str("subject", subject).
			Msg("🔄 ZED_FLOW_DEBUG: [STEP 2.7] NATS consumer message loop started - waiting for messages from JetStream")

		for {
			select {
			case <-ctx.Done():
				log.Debug().
					Str("EXTERNAL_AGENT_DEBUG", "consumer_loop_cancelled").
					Str("stream", stream).
					Str("subject", subject).
					Msg("🛑 EXTERNAL_AGENT_DEBUG: Consumer message loop cancelled")
				return
			default:
				log.Trace().
					Str("ZED_FLOW_DEBUG", "fetching_next_nats_message").
					Str("stream", stream).
					Str("subject", subject).
					Msg("🔍 ZED_FLOW_DEBUG: Polling JetStream for next message...")

				msg, err := mc.Next()
				if err != nil {
					log.Err(err).
						Str("ZED_FLOW_DEBUG", "nats_fetch_error").
						Str("stream", stream).
						Str("subject", subject).
						Msg("❌ ZED_FLOW_DEBUG: Failed to fetch message from JetStream - retrying in 1s")
					time.Sleep(1 * time.Second) // Prevent tight loop on connection errors
					continue
				}

				log.Info().
					Str("ZED_FLOW_DEBUG", "nats_message_received").
					Str("stream", stream).
					Str("subject", subject).
					Str("msg_subject", msg.Subject()).
					Str("reply_header", msg.Headers().Get(helixNatsReplyHeader)).
					Str("subject_header", msg.Headers().Get(helixNatsSubjectHeader)).
					Int("data_length", len(msg.Data())).
					Msg("🎯 ZED_FLOW_DEBUG: [STEP 2.8] NATS message received from JetStream - about to call handler")

				err = handler(&Message{
					Type:   msg.Headers().Get(helixNatsSubjectHeader),
					Reply:  msg.Headers().Get(helixNatsReplyHeader),
					Data:   msg.Data(),
					Header: msg.Headers(),
					msg:    msg,
				})
				if err != nil {
					log.Err(err).
						Str("ZED_FLOW_DEBUG", "handler_error").
						Str("stream", stream).
						Str("subject", subject).
						Msg("❌ ZED_FLOW_DEBUG: [STEP 2.9 FAILED] Handler function returned error")
				} else {
					log.Info().
						Str("ZED_FLOW_DEBUG", "handler_success").
						Str("stream", stream).
						Str("subject", subject).
						Msg("✅ ZED_FLOW_DEBUG: [STEP 2.9] Handler function completed successfully - message forwarded to WebSocket")
				}
			}
		}

	}()

	// Consumer wrapper noop
	return &consumerWrapper{}, nil
}

func (n *Nats) NumSubscriptions() int {
	return n.conn.NumSubscriptions()
}

func (n *Nats) Close() {
	n.conn.Close()
}

type consumerWrapper struct{}

func (c *consumerWrapper) Unsubscribe() error {
	return nil
}

// gcJetStream is a helper function to clean up old streams
func gcJetStream(js jetstream.JetStream) {
	streams := js.ListStreams(context.Background())

	for s := range streams.Info() {
		log.Debug().
			Str("name", s.Config.Name).
			Strs("subjects", s.Config.Subjects).
			Msg("checking stream for cleanup")

		// Clean up SCRIPTS.* streams (legacy cleanup)
		if len(s.Config.Subjects) > 0 && s.Config.Subjects[0] == "SCRIPTS.*" {
			if err := js.DeleteStream(context.Background(), s.Config.Name); err != nil {
				log.Err(err).Str("name", s.Config.Name).Msg("failed to delete stream")
			}
		}

		// Clean up ZED_AGENTS.* streams and their consumers to prevent conflicts
		if len(s.Config.Subjects) > 0 && s.Config.Subjects[0] == "ZED_AGENTS.*" {
			// First, get the stream to list its consumers
			if stream, err := js.Stream(context.Background(), s.Config.Name); err == nil {
				consumers := stream.ListConsumers(context.Background())
				for c := range consumers.Info() {
					log.Debug().
						Str("stream", s.Config.Name).
						Str("consumer", c.Name).
						Msg("deleting consumer for cleanup")
					if err := js.DeleteConsumer(context.Background(), s.Config.Name, c.Name); err != nil {
						log.Err(err).
							Str("stream", s.Config.Name).
							Str("consumer", c.Name).
							Msg("failed to delete consumer during cleanup")
					}
				}
			}

			// Then delete the stream
			if err := js.DeleteStream(context.Background(), s.Config.Name); err != nil {
				log.Err(err).Str("name", s.Config.Name).Msg("failed to delete ZED_AGENTS stream")
			}
		}
	}
}

func checkStoreDir(dir string) error {
	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// Try to create it
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create store directory: %w", err)
		}
	}

	// Check if directory is writable
	testFile := filepath.Join(dir, ".write_test")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("directory not writable: %w", err)
	}
	f.Close()
	os.Remove(testFile)

	return nil
}
