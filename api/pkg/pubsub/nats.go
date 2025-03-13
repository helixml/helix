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
	scriptStreamName       = "SCRIPTS_STREAM"
	scriptsSubject         = "SCRIPTS.*"
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
		return nil, fmt.Errorf("failed to create jetstream context: %w", err)
	}

	// Clean up old streams
	gcJetStream(js)

	stream, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:      scriptStreamName,
		Subjects:  []string{scriptsSubject},
		Retention: jetstream.WorkQueuePolicy,
		// Storage:   jetstream.MemoryStorage,
		Discard: jetstream.DiscardOld,
		MaxAge:  5 * time.Minute, // Discard messages older than 5 minutes
		// ConsumerLimits: jetstream.StreamConsumerLimits{
		// 	MaxAckPending: 20,
		// },
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create internal jetstream stream: %w", err)
	}

	ctx := context.Background()
	c, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		AckPolicy:      jetstream.AckExplicitPolicy,
		FilterSubjects: []string{getStreamSub(ScriptRunnerStream, AppQueue)},
		AckWait:        5 * time.Second,
		// MemoryStorage:  true,
		ReplayPolicy: jetstream.ReplayInstantPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	// Basic monitoring of the stream
	go func() {
		for {
			info, err := stream.Info(ctx)
			if err != nil {
				log.Err(err).Msg("failed to get stream info")
				continue
			}
			if info.State.Consumers == 0 {
				log.Debug().
					Int("messages", int(info.State.Msgs)).
					Int("consumers", info.State.Consumers).
					Time("oldest_message", info.State.FirstTime).
					Time("newest_message", info.State.LastTime).
					Msg("Stream info")
			}

			time.Sleep(10 * time.Second)
		}
	}()

	n := &Nats{
		conn:           nc,
		embeddedServer: ns,
		js:             js,
		stream:         stream,
		consumer:       c,
		statusHandlers: make([]ConnectionStatusHandler, 0),
	}

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
		nats.Timeout(time.Second * 2),
		nats.RetryOnFailedConnect(false),
		nats.MaxReconnects(-1), // Infinite reconnects
		nats.ReconnectWait(time.Second * 2),
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

	// Publish the message to the JetStream stream,
	// one of the consumer will pick it up
	_, err = n.js.PublishMsg(ctx, &nats.Msg{
		Subject: streamTopic,
		Data:    payload,
		Header:  hdr,
	},
		jetstream.WithRetryWait(50*time.Millisecond),
		jetstream.WithRetryAttempts(10),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to publish message to jetstream: %w", err)
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
	n.consumerMu.Lock()
	defer n.consumerMu.Unlock()

	info, err := n.stream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	if info.State.Consumers == 0 {
		// Creating consumer
		ctx := context.Background()
		c, err := n.stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			AckPolicy:      jetstream.AckExplicitPolicy,
			FilterSubjects: []string{getStreamSub(stream, subject)},
			AckWait:        5 * time.Second,
			ReplayPolicy:   jetstream.ReplayInstantPolicy,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create consumer: %w", err)
		}
		n.consumer = c
	}

	mc, err := n.consumer.Messages()
	if err != nil {
		return nil, fmt.Errorf("failed to get messages context: %w", err)
	}

	go func() {
		<-ctx.Done()
		mc.Stop()
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := mc.Next()
				if err != nil {
					log.Err(err).Msg("failed to fetch messages")
					continue
				}

				err = handler(&Message{
					Type:   msg.Headers().Get(helixNatsSubjectHeader),
					Reply:  msg.Headers().Get(helixNatsReplyHeader),
					Data:   msg.Data(),
					Header: msg.Headers(),
					msg:    msg,
				})
				if err != nil {
					log.Err(err).Msg("error handling message")
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
		if s.Config.Subjects[0] == "SCRIPTS.*" {
			if err := js.DeleteStream(context.Background(), s.Config.Name); err != nil {
				log.Err(err).Str("name", s.Config.Name).Msg("failed to delete stream")
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
