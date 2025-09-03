package zedagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"

	"net"

	"github.com/helixml/helix/api/pkg/config"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	retries             = 100
	delayBetweenRetries = 3 * time.Second
)

// ZedRunner connects to Control Plane and processes Zed agent tasks
// Replaces GPTScript runner entirely - each runner handles one session then exits
type ZedRunner struct {
	cfg            *config.GPTScriptRunnerConfig
	zedExecutor    *external_agent.ZedExecutor
	rdpConn        net.Conn        // Connection to local RDP server for proxy
	currentSession string          // Track current session for RDP routing
	pubsub         PubSubInterface // For RDP data routing
	apiConn        *websocket.Conn // WebSocket connection to API
}

// PubSubInterface defines pub/sub operations needed for RDP routing
type PubSubInterface interface {
	Subscribe(ctx context.Context, topic string, handler func(payload []byte) error) (Subscription, error)
	Publish(ctx context.Context, topic string, payload []byte) error
}

// Subscription interface for managing subscriptions
type Subscription interface {
	Unsubscribe() error
}

func NewZedRunner(cfg *config.GPTScriptRunnerConfig, zedExecutor *external_agent.ZedExecutor, pubsub PubSubInterface) *ZedRunner {
	return &ZedRunner{
		cfg:         cfg,
		zedExecutor: zedExecutor,
		pubsub:      pubsub,
	}
}

func (r *ZedRunner) Run(ctx context.Context) error {
	log.Info().
		Str("runner_id", r.cfg.RunnerID).
		Int("concurrency", r.cfg.Concurrency).
		Int("max_tasks", r.cfg.MaxTasks).
		Msg("Starting Zed runner")

	return r.run(ctx)
}

func (r *ZedRunner) run(ctx context.Context) error {
	var conn *websocket.Conn

	err := retry.Do(
		func() error {
			var err error
			conn, err = r.dial(ctx)
			if err != nil {
				return err
			}
			return nil
		},
		retry.Context(ctx),
		retry.Attempts(retries),
		retry.Delay(delayBetweenRetries),
	)

	if err != nil {
		return fmt.Errorf("failed to connect to control plane after %d retries: %w", retries, err)
	}

	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	// Store the API connection for RDP data routing
	r.apiConn = conn

	// Graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Message processing pool
	p := pool.New().
		WithMaxGoroutines(r.cfg.Concurrency).
		WithContext(ctx).
		WithCancelOnError()

	var ops atomic.Uint64

	p.Go(func(ctx context.Context) error {
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			mt, message, err := conn.ReadMessage()
			if err != nil {
				if strings.Contains(err.Error(), "broken pipe") {
					return fmt.Errorf("control plane connection lost: %w", err)
				}
				return fmt.Errorf("failed to read websocket message: %w", err)
			}

			// Handle both text messages (task requests) and binary messages (RDP proxy data)
			switch mt {
			case websocket.TextMessage:
				// Process Zed agent task requests
				p.Go(func(ctx context.Context) error {
					if err := r.processMessage(ctx, conn, message); err != nil {
						log.Err(err).Msg("failed to process Zed agent message")
						return err
					}
					ops.Add(1)

					// Exit after completing one task (container will restart for cleanup)
					if r.cfg.MaxTasks > 0 && ops.Load() >= uint64(r.cfg.MaxTasks) {
						log.Info().
							Uint64("completed_tasks", ops.Load()).
							Msg("Zed runner completed task, exiting for container restart")
						cancel()
					}
					return nil
				})

			case websocket.BinaryMessage:
				// Handle RDP proxy data - forward to local RDP server in container
				p.Go(func(ctx context.Context) error {
					return r.handleRDPProxyData(ctx, conn, message)
				})
			}
		}
	})

	// Keepalive ping
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			err := conn.WriteMessage(websocket.PingMessage, []byte{})
			if err != nil {
				if strings.Contains(err.Error(), "broken pipe") {
					return fmt.Errorf("control plane closed connection, restarting: %s", err)
				}
				log.Err(err).Msg("failed to send ping, closing connection")
				return fmt.Errorf("failed to send ping (%w), closing connection", err)
			}
		}
	}
}

func (r *ZedRunner) dial(ctx context.Context) (*websocket.Conn, error) {
	var apiHost string

	if strings.HasPrefix(r.cfg.APIHost, "https://") {
		apiHost = strings.Replace(r.cfg.APIHost, "https", "wss", 1)
	}
	if strings.HasPrefix(r.cfg.APIHost, "http://") {
		apiHost = strings.Replace(r.cfg.APIHost, "http", "ws", 1)
	}

	// Connect to Zed runner endpoint (not gptscript)
	apiHost = fmt.Sprintf("%s%s?access_token=%s&concurrency=%d&runnerid=%s",
		apiHost,
		system.GetAPIPath("/ws/zed-runner"),
		url.QueryEscape(r.cfg.APIToken),
		r.cfg.Concurrency,
		r.cfg.RunnerID,
	)

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, apiHost, nil)
	if err != nil {
		log.Error().
			Str("api_host", apiHost).
			Str("runner_id", r.cfg.RunnerID).
			Err(err).
			Msg("websocket dial failed")
		return nil, fmt.Errorf("websocket dial to '%s' failed: %w", apiHost, err)
	}

	log.Info().
		Str("runner_id", r.cfg.RunnerID).
		Msg("🟢 Zed runner connected to control plane")

	return conn, nil
}

func (r *ZedRunner) processMessage(ctx context.Context, conn *websocket.Conn, message []byte) error {
	var envelope types.RunnerEventRequestEnvelope
	if err := json.Unmarshal(message, &envelope); err != nil {
		return fmt.Errorf("failed to unmarshal message envelope: %w", err)
	}

	switch envelope.Type {
	case "zed_agent_request":
		return r.processZedAgentRequest(ctx, conn, &envelope)
	default:
		return fmt.Errorf("unknown message type: %s", envelope.Type)
	}
}

func (r *ZedRunner) processZedAgentRequest(ctx context.Context, conn *websocket.Conn, req *types.RunnerEventRequestEnvelope) error {
	logger := log.With().
		Str("request_id", req.RequestID).
		Str("runner_id", r.cfg.RunnerID).
		Logger()

	var agent types.ZedAgent
	if err := json.Unmarshal(req.Payload, &agent); err != nil {
		logger.Err(err).Msgf("failed to unmarshal Zed agent: %s", string(req.Payload))
		return fmt.Errorf("failed to unmarshal Zed agent: %w", err)
	}

	logger.Info().
		Str("session_id", agent.SessionID).
		Str("user_id", agent.UserID).
		Str("input", agent.Input).
		Str("project_path", agent.ProjectPath).
		Str("work_dir", agent.WorkDir).
		Int("env_vars", len(agent.Env)).
		Msg("Processing Zed agent request")

	start := time.Now()

	// Use the Zed executor to start the agent
	// Start the Zed agent
	resp, err := r.zedExecutor.StartZedAgent(ctx, &agent)
	if err != nil {
		logger.Error().Err(err).Msg("Zed agent execution failed")
		resp = &types.ZedAgentResponse{
			SessionID: agent.SessionID,
			Error:     err.Error(),
			Status:    "error",
		}
	} else {
		// Set current session for RDP routing
		r.currentSession = agent.SessionID

		// Establish connection to local RDP server for proxy
		if err := r.connectToLocalRDP(ctx); err != nil {
			logger.Error().Err(err).Msg("Failed to connect to local RDP server")
		} else {
			logger.Info().Msg("Connected to local RDP server for proxy")
		}

		// Subscribe to RDP commands for this session
		if err := r.subscribeToRDPCommands(ctx, agent.SessionID); err != nil {
			logger.Error().Err(err).Msg("Failed to subscribe to RDP commands")
		} else {
			logger.Info().Msg("Subscribed to RDP commands via NATS")
		}

		logger.Info().
			Str("session_id", agent.SessionID).
			Str("rdp_url", resp.RDPURL).
			TimeDiff("duration", time.Now(), start).
			Msg("Zed agent started successfully with RDP proxy")
	}

	// Send response back to control plane
	return r.respond(conn, req.RequestID, req.Reply, resp)
}

func (r *ZedRunner) respond(conn *websocket.Conn, reqID, reply string, resp interface{}) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	envelope := types.RunnerEventResponseEnvelope{
		RequestID: reqID,
		Reply:     reply,
		Payload:   payload,
	}

	responseBytes, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("failed to marshal response envelope: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, responseBytes); err != nil {
		return fmt.Errorf("failed to write response message: %w", err)
	}

	log.Debug().
		Str("request_id", reqID).
		Str("reply", reply).
		Msg("Response sent to control plane")

	return nil
}

// Initialize Zed runner with environment configuration for scaling
func InitializeZedRunner(pubsub PubSubInterface) (*ZedRunner, error) {
	// Generate unique runner ID for scaling support
	runnerID := generateUniqueRunnerID()

	cfg := &config.GPTScriptRunnerConfig{
		APIHost:     getEnvOrDefault("API_HOST", "http://localhost:8080"),
		APIToken:    getEnvOrDefault("API_TOKEN", ""),
		RunnerID:    runnerID,
		Concurrency: getIntEnvOrDefault("CONCURRENCY", 1),
		MaxTasks:    getIntEnvOrDefault("MAX_TASKS", 1),
	}

	// Validate required configuration
	if cfg.APIToken == "" {
		return nil, fmt.Errorf("API_TOKEN environment variable is required")
	}

	// Initialize Zed executor with container isolation pattern
	// Each container is isolated so we use fixed display :1 and port 5900
	workspaceDir := getEnvOrDefault("WORKSPACE_DIR", "/tmp/workspace")
	zedExecutor := external_agent.NewZedExecutor(workspaceDir)

	log.Info().
		Str("runner_id", cfg.RunnerID).
		Str("api_host", cfg.APIHost).
		Str("workspace_dir", workspaceDir).
		Msg("Zed runner initialized with container isolation")

	return NewZedRunner(cfg, zedExecutor, pubsub), nil
}

// handleRDPProxyData forwards RDP data between API and local RDP server
func (r *ZedRunner) handleRDPProxyData(ctx context.Context, conn *websocket.Conn, data []byte) error {
	if r.rdpConn == nil {
		log.Debug().Msg("No RDP connection available, ignoring proxy data")
		return nil
	}

	// Forward binary data to local RDP server
	r.rdpConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := r.rdpConn.Write(data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to forward RDP data to local server")
		return err
	}

	log.Debug().
		Int("data_size", len(data)).
		Msg("Forwarded RDP data to local server")

	return nil
}

// connectToLocalRDP establishes connection to the local RDP server in this container
func (r *ZedRunner) connectToLocalRDP(ctx context.Context) error {
	// Connect to local RDP server (xrdp running on port 5900 in same container)
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	conn, err := dialer.DialContext(ctx, "tcp", "localhost:5900")
	if err != nil {
		return fmt.Errorf("failed to connect to local RDP server: %w", err)
	}

	r.rdpConn = conn

	// Start reading RDP data from local server and send back to API
	go r.proxyRDPDataToAPI(ctx)

	log.Info().Msg("Connected to local RDP server for proxy")
	return nil
}

// proxyRDPDataToAPI reads RDP data from local server and sends to API via WebSocket
func (r *ZedRunner) proxyRDPDataToAPI(ctx context.Context) {
	defer func() {
		if r.rdpConn != nil {
			r.rdpConn.Close()
			r.rdpConn = nil
		}
	}()

	buffer := make([]byte, 8192)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		r.rdpConn.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := r.rdpConn.Read(buffer)
		if err != nil {
			log.Error().Err(err).Msg("RDP connection to local server closed")
			return
		}

		if n > 0 {
			// Send RDP data back to API via WebSocket
			// The API will convert this to Guacamole protocol for the frontend
			if err := r.sendRDPDataToAPI(buffer[:n]); err != nil {
				log.Error().Err(err).Msg("Failed to send RDP data to API")
				return
			}
		}
	}
}

// sendRDPDataToAPI sends RDP data to the API via WebSocket
func (r *ZedRunner) sendRDPDataToAPI(data []byte) error {
	// Create response envelope with RDP data
	response := types.ZedAgentRDPData{
		SessionID: r.currentSession,
		Type:      "rdp_data",
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	payload, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal RDP data: %w", err)
	}

	envelope := types.RunnerEventResponseEnvelope{
		RequestID: fmt.Sprintf("rdp-%s-%d", r.currentSession, time.Now().UnixNano()),
		Reply:     fmt.Sprintf("/sessions/%s/rdp-data", r.currentSession),
		Payload:   payload,
	}

	responseBytes, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("failed to marshal response envelope: %w", err)
	}

	// Send via WebSocket to API
	if err := r.sendToAPI(websocket.BinaryMessage, responseBytes); err != nil {
		return fmt.Errorf("failed to send RDP data to API: %w", err)
	}

	return nil
}

// sendToAPI sends data to the API via WebSocket
func (r *ZedRunner) sendToAPI(messageType int, data []byte) error {
	if r.apiConn == nil {
		return fmt.Errorf("no API connection available")
	}

	r.apiConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := r.apiConn.WriteMessage(messageType, data)
	if err != nil {
		return fmt.Errorf("failed to send data to API: %w", err)
	}

	log.Debug().
		Int("message_type", messageType).
		Int("data_size", len(data)).
		Msg("Sent RDP data to API via WebSocket")

	return nil
}

// subscribeToRDPCommands subscribes to RDP commands for the current session
func (r *ZedRunner) subscribeToRDPCommands(ctx context.Context, sessionID string) error {
	if r.pubsub == nil {
		return fmt.Errorf("no pubsub interface available")
	}

	rdpTopic := fmt.Sprintf("rdp.commands.%s", sessionID)

	_, err := r.pubsub.Subscribe(ctx, rdpTopic, func(payload []byte) error {
		// Handle RDP commands from API
		return r.handleRDPCommandFromAPI(payload)
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to RDP commands: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("topic", rdpTopic).
		Msg("Subscribed to RDP commands via NATS")

	return nil
}

// handleRDPCommandFromAPI processes RDP commands received via NATS from API
func (r *ZedRunner) handleRDPCommandFromAPI(payload []byte) error {
	var rdpData types.ZedAgentRDPData
	if err := json.Unmarshal(payload, &rdpData); err != nil {
		return fmt.Errorf("failed to unmarshal RDP command: %w", err)
	}

	if rdpData.SessionID != r.currentSession {
		log.Warn().
			Str("expected_session", r.currentSession).
			Str("received_session", rdpData.SessionID).
			Msg("Received RDP command for wrong session")
		return nil
	}

	// Forward RDP data to local RDP server
	if r.rdpConn != nil {
		r.rdpConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, err := r.rdpConn.Write(rdpData.Data)
		if err != nil {
			return fmt.Errorf("failed to forward RDP data to local server: %w", err)
		}

		log.Debug().
			Str("session_id", rdpData.SessionID).
			Int("data_size", len(rdpData.Data)).
			Msg("Forwarded RDP command to local server")
	}

	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnvOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := parseIntWithDefault(value, defaultValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func parseIntWithDefault(s string, defaultValue int) (int, error) {
	if s == "" {
		return defaultValue, nil
	}

	var result int
	if _, err := fmt.Sscanf(s, "%d", &result); err != nil {
		return defaultValue, err
	}
	return result, nil
}

// generateUniqueRunnerID creates a unique runner ID for scaling
func generateUniqueRunnerID() string {
	// Use container hostname + timestamp for uniqueness
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "zed-runner"
	}

	// Clean hostname to be valid runner ID
	runnerID := fmt.Sprintf("%s-%d", hostname, time.Now().Unix())

	log.Info().
		Str("runner_id", runnerID).
		Str("hostname", hostname).
		Msg("Generated unique runner ID for scaling")

	return runnerID
}
