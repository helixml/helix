package pubsub

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
)

type ConnectionStatusHandler func(status ConnectionStatus)

type Nats struct {
	conn           *nats.Conn
	js             jetstream.JetStream
	embeddedServer *server.Server

	// ownedStoreDir is the JetStream store directory NewInMemoryNats
	// created; Close removes it. Empty for configured store directories.
	ownedStoreDir string
	stopMonitor   context.CancelFunc

	consumerMu sync.Mutex
	consumer   jetstream.Consumer

	// Zed agent consumer (separate from GPTScript)
	zedConsumer jetstream.Consumer

	streams map[string]jetstream.Stream

	statusHandlers []ConnectionStatusHandler
	statusMu       sync.RWMutex
}

func (n *Nats) OnConnectionStatus(handler ConnectionStatusHandler) {
	n.statusMu.Lock()
	defer n.statusMu.Unlock()
	n.statusHandlers = append(n.statusHandlers, handler)
}

func (n *Nats) notifyStatusChange(status ConnectionStatus) {
	// Call handlers outside the lock: one may register another handler.
	n.statusMu.RLock()
	handlers := slices.Clone(n.statusHandlers)
	n.statusMu.RUnlock()
	for _, handler := range handlers {
		handler(status)
	}
}

func setupConnectionHandlers(nc *nats.Conn, n *Nats) {
	nc.SetDisconnectErrHandler(func(_ *nats.Conn, err error) {
		log.Warn().Err(err).Msg("nats connection lost")
		n.notifyStatusChange(Disconnected)
	})

	nc.SetClosedHandler(func(_ *nats.Conn) {
		log.Warn().Msg("nats connection closed")
		n.notifyStatusChange(Disconnected)
	})

	nc.SetDiscoveredServersHandler(func(_ *nats.Conn) {
		log.Debug().Strs("servers", nc.DiscoveredServers()).Msg("discovered nats servers")
	})

	// The reconnect handler fires once the connection is re-established.
	nc.SetReconnectHandler(func(_ *nats.Conn) {
		log.Info().Msg("nats reconnected")
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

	// Fall back to the runner token if no token is provided
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
		if ns != nil {
			ns.Shutdown()
			ns.WaitForShutdown()
		}
		return nil, fmt.Errorf("failed to connect to nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		if ns != nil {
			ns.Shutdown()
			ns.WaitForShutdown()
		}
		return nil, fmt.Errorf("failed to create jetstream: %w", err)
	}

	// Initialize Nats struct early so we can use n variable
	n := &Nats{
		conn:           nc,
		embeddedServer: ns,
		js:             js,
		streams:        make(map[string]jetstream.Stream),
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
		n.Close()
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
		n.Close()
		return nil, fmt.Errorf("failed to create script runner stream: %w", err)
	}

	// Store streams in map
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
			n.Close()
			return nil, fmt.Errorf("failed to create consumer: %w", err)
		}

		log.Info().
			Str("consumer", consumerName).
			Str("stream", zedAgentStreamName).
			Str("filter_subject", getStreamSub(ZedAgentRunnerStream, ZedAgentQueue)).
			Msg("successfully created ZED_AGENTS consumer")
	}
	n.consumer = c

	// Basic monitoring of the stream, stopped by Close.
	monitorCtx, stopMonitor := context.WithCancel(context.Background())
	n.stopMonitor = stopMonitor
	go func() {
		for {
			wait := 10 * time.Second
			info, err := stream.Info(monitorCtx)
			if monitorCtx.Err() != nil {
				return
			}
			if err != nil {
				log.Err(err).Msg("failed to get stream info")
				wait = time.Second
			} else if info.State.Consumers == 0 {
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

			select {
			case <-monitorCtx.Done():
				return
			case <-time.After(wait):
			}
		}
	}()

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

	cfg := &config.ServerConfig{}
	cfg.PubSub.StoreDir = tmpDir
	cfg.PubSub.Server.Host = "127.0.0.1" // no token: never reachable off-host
	// Let the kernel assign both ports at bind time. Probing for a free port
	// first (and assuming port+1 is free too) races with every other
	// in-process server; a lost race costs ReadyForConnections' full 4s.
	cfg.PubSub.Server.Port = server.RANDOM_PORT
	cfg.PubSub.Server.WebsocketPort = server.RANDOM_PORT
	cfg.PubSub.Server.JetStream = true
	cfg.PubSub.Server.MaxPayload = 32 * 1024 * 1024 // 32MB
	cfg.PubSub.Server.EmbeddedNatsServerEnabled = true

	n, err := NewNats(cfg)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, err
	}
	n.ownedStoreDir = tmpDir
	return n, nil
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
			log.Err(err).Str("subject", msg.Subject).Msg("error handling message")
		}
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe %q: %w", topic, err)
	}

	return sub, nil
}

func (n *Nats) SubscribeWithCtx(_ context.Context, topic string, handler func(ctx context.Context, msg *nats.Msg) error) (Subscription, error) {
	sub, err := n.conn.Subscribe(topic, func(msg *nats.Msg) {
		err := handler(context.Background(), msg)
		if err != nil {
			log.Err(err).Str("subject", msg.Subject).Msg("error handling message")
		}
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe %q: %w", topic, err)
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
	// Buffered with a non-blocking send: a reply arriving after the caller
	// gave up must not block the subscription's delivery goroutine forever.
	dataCh := make(chan []byte, 1)

	sub, err := n.conn.Subscribe(replyInbox, func(msg *nats.Msg) {
		select {
		case dataCh <- msg.Data:
		default:
		}
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe reply inbox: %w", err)
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
			log.Err(err).Str("subject", msg.Subject).Str("queue", queue).Msg("error handling message")
		}
	})
	if err != nil {
		return nil, fmt.Errorf("queue subscribe %q on %q: %w", queue, subject, err)
	}

	return sub, nil
}

// StreamRequest publishes a message to the stream and creates an inbox to receive the response. If response is not
// received within the timeout, an error is returned.
func (n *Nats) StreamRequest(ctx context.Context, stream, subject string, payload []byte, header map[string]string, timeout time.Duration) ([]byte, error) {
	replyInbox := nats.NewInbox()
	// Buffered with a non-blocking send: a reply arriving after the caller
	// gave up must not block the subscription's delivery goroutine forever.
	dataCh := make(chan []byte, 1)

	sub, err := n.conn.Subscribe(replyInbox, func(msg *nats.Msg) {
		select {
		case dataCh <- msg.Data:
		default:
		}
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe reply inbox: %w", err)
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

	streamTopic := getStreamSub(stream, subject)

	// One of the stream's consumers will pick it up.
	ack, err := n.js.PublishMsg(ctx, &nats.Msg{
		Subject: streamTopic,
		Data:    payload,
		Header:  hdr,
	},
		jetstream.WithRetryWait(50*time.Millisecond),
		jetstream.WithRetryAttempts(10),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to publish message to jetstream subject %s: %w", streamTopic, err)
	}
	log.Debug().
		Str("stream", ack.Stream).
		Str("subject", streamTopic).
		Uint64("sequence", ack.Sequence).
		Bool("duplicate", ack.Duplicate).
		Msg("published request to jetstream")

	select {
	case <-ctx.Done():
		target, ok := n.streams[stream]
		if !ok {
			return nil, ctx.Err()
		}
		// The caller's context is done; bound the diagnostic on its own.
		infoCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		info, err := target.Info(infoCtx)
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

// StreamConsume delivers messages from a JetStream stream to handler. All
// subscribers of a stream share one consumer, so each message reaches one of
// them. The subscription stops on Unsubscribe, when ctx ends, or when the
// iterator fails terminally (e.g. the consumer is deleted).
func (n *Nats) StreamConsume(ctx context.Context, stream, subject string, handler func(msg *Message) error) (Subscription, error) {
	n.consumerMu.Lock()
	defer n.consumerMu.Unlock()

	targetStream, ok := n.streams[stream]
	if !ok {
		return nil, fmt.Errorf("stream %s not found", stream)
	}

	info, err := targetStream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	// Only create a consumer if the stream has none; otherwise share it.
	if info.State.Consumers == 0 {
		// Anonymous (no Name): a named one fails with "not unique" when
		// another process created it concurrently.
		c, err := targetStream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			AckPolicy:      jetstream.AckExplicitPolicy,
			FilterSubjects: []string{getStreamSub(stream, subject)},
			AckWait:        5 * time.Second,
			BackOff:        []time.Duration{1 * time.Second, 5 * time.Second, 30 * time.Second},
			ReplayPolicy:   jetstream.ReplayInstantPolicy,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create consumer: %w", err)
		}
		if stream == ZedAgentRunnerStream {
			n.zedConsumer = c
		} else {
			n.consumer = c
		}
	}

	var activeConsumer jetstream.Consumer
	if stream == ZedAgentRunnerStream {
		activeConsumer = n.zedConsumer
	} else {
		activeConsumer = n.consumer
	}

	// No local reference yet: adopt the consumer created at startup.
	if activeConsumer == nil {
		consumers := targetStream.ListConsumers(ctx)
		var startup *jetstream.ConsumerInfo
		for consumerInfo := range consumers.Info() {
			if startup == nil {
				startup = consumerInfo
			}
		}
		if err := consumers.Err(); err != nil {
			return nil, fmt.Errorf("failed to list consumers of stream %s: %w", stream, err)
		}
		if startup == nil {
			return nil, fmt.Errorf("stream %s has no consumer to read from", stream)
		}
		consumer, err := targetStream.Consumer(ctx, startup.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get consumer %s: %w", startup.Name, err)
		}
		if stream == ZedAgentRunnerStream {
			n.zedConsumer = consumer
		} else {
			n.consumer = consumer
		}
		activeConsumer = consumer
	}

	// Missing heartbeats are transient (a stalled link); the iterator recovers
	// on its own, so only terminal errors may end the loop.
	mc, err := activeConsumer.Messages(jetstream.WithMessagesErrOnMissingHeartbeat(false))
	if err != nil {
		return nil, fmt.Errorf("failed to get messages context: %w", err)
	}

	sub := &messagesSubscription{messages: mc, done: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			sub.Unsubscribe()
		case <-sub.done:
		}
	}()

	go func() {
		defer sub.Unsubscribe()
		for {
			msg, err := mc.Next()
			if err != nil {
				select {
				case <-sub.done:
				default:
					log.Error().Err(err).Str("stream", stream).Str("subject", subject).Msg("stream consumer stopped")
				}
				return
			}

			log.Debug().
				Str("stream", stream).
				Str("msg_subject", msg.Subject()).
				Str("reply_header", msg.Headers().Get(helixNatsReplyHeader)).
				Int("data_length", len(msg.Data())).
				Msg("nats stream message received")

			err = handler(&Message{
				Type:   msg.Headers().Get(helixNatsSubjectHeader),
				Reply:  msg.Headers().Get(helixNatsReplyHeader),
				Data:   msg.Data(),
				Header: msg.Headers(),
				msg:    msg,
			})
			if err != nil {
				log.Err(err).Str("stream", stream).Str("subject", subject).Msg("stream consumer handler failed")
			}
		}
	}()

	return sub, nil
}

// messagesSubscription stops a JetStream message iterator exactly once.
type messagesSubscription struct {
	messages jetstream.MessagesContext
	done     chan struct{}
	once     sync.Once
}

func (s *messagesSubscription) Unsubscribe() error {
	s.once.Do(func() {
		close(s.done)
		s.messages.Stop()
	})
	return nil
}

func (n *Nats) NumSubscriptions() int {
	return n.conn.NumSubscriptions()
}

// Close releases everything this instance owns: the stream monitor, the
// connection, the embedded server and a store directory it created.
func (n *Nats) Close() {
	if n.stopMonitor != nil {
		n.stopMonitor()
	}
	n.conn.Close()
	if n.embeddedServer != nil {
		n.embeddedServer.Shutdown()
		n.embeddedServer.WaitForShutdown()
	}
	if n.ownedStoreDir != "" {
		if err := os.RemoveAll(n.ownedStoreDir); err != nil {
			log.Warn().Err(err).Str("dir", n.ownedStoreDir).Msg("failed to remove nats store dir")
		}
	}
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
