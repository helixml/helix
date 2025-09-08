package external_agent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	retries             = 100
	delayBetweenRetries = 3 * time.Second
)

// ExternalAgentRunner connects using a WebSocket to the Control Plane
// and listens for external agent tasks to run (follows GPTScript runner pattern)
type ExternalAgentRunner struct {
	cfg            *config.ExternalAgentRunnerConfig
	rdpConnections map[string]*RDPConnection // connectionKey -> RDP connection (supports both runnerID and sessionID keys)
	rdpMutex       sync.RWMutex
}

// RDPConnection represents a connection to local XRDP server
type RDPConnection struct {
	connectionKey  string // Either sessionID or runnerID
	connectionType string // "session" or "runner"
	runnerID       string // Always set to identify which runner this is
	conn           net.Conn
	replyTopic     string
	websocketConn  *websocket.Conn
	ctx            context.Context
	cancel         context.CancelFunc
}

func NewExternalAgentRunner(cfg *config.ExternalAgentRunnerConfig) *ExternalAgentRunner {
	return &ExternalAgentRunner{
		cfg:            cfg,
		rdpConnections: make(map[string]*RDPConnection),
	}
}

func (r *ExternalAgentRunner) Run(ctx context.Context) error {
	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "runner_start").
		Str("api_host", r.cfg.APIHost).
		Str("api_token", r.cfg.APIToken).
		Str("runner_id", r.cfg.RunnerID).
		Int("concurrency", r.cfg.Concurrency).
		Int("max_tasks", r.cfg.MaxTasks).
		Msg("🚀 EXTERNAL_AGENT_DEBUG: Starting external agent runner")
	return r.run(ctx)
}

func (r *ExternalAgentRunner) run(ctx context.Context) error {
	var conn *websocket.Conn

	err := retry.Do(func() error {
		var err error
		conn, err = r.dial(ctx)
		if err != nil {
			return err
		}
		return nil
	},
		retry.Attempts(retries),
		retry.Delay(delayBetweenRetries),
		retry.Context(ctx),
		retry.OnRetry(func(n uint, err error) {
			log.Warn().
				Err(err).
				Uint("retries", n).
				Msg("retrying to connect to control plane")
		}),
	)
	if err != nil {
		return err
	}

	defer conn.Close()

	done := make(chan struct{})

	pool := pool.New().WithMaxGoroutines(r.cfg.Concurrency)
	var ops atomic.Uint64

	ctx, cancel := context.WithCancel(ctx)

	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "task_processing_start").
		Int("concurrency", r.cfg.Concurrency).
		Int("max_tasks", r.cfg.MaxTasks).
		Msg("🟢 EXTERNAL_AGENT_DEBUG: Starting external agent task processing")

	go func() {
		defer close(done)
		for {
			mt, message, err := conn.ReadMessage()
			if err != nil {
				if errors.Is(ctx.Err(), context.Canceled) {
					log.Info().
						Str("EXTERNAL_AGENT_DEBUG", "context_cancelled").
						Msg("🟠 EXTERNAL_AGENT_DEBUG: Context cancelled, stopping message processing")
					return
				}
				log.Error().
					Str("EXTERNAL_AGENT_DEBUG", "websocket_read_error").
					Err(err).
					Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to read websocket message")
				return
			}

			if mt != websocket.TextMessage {
				log.Debug().
					Str("EXTERNAL_AGENT_DEBUG", "non_text_message").
					Int("message_type", int(mt)).
					Msg("🔄 EXTERNAL_AGENT_DEBUG: Received non-text message, skipping")
				continue
			}

			log.Info().
				Str("EXTERNAL_AGENT_DEBUG", "message_received").
				Int("message_length", len(message)).
				Str("message_preview", func() string {
					if len(message) > 200 {
						return string(message[:200]) + "..."
					}
					return string(message)
				}()).
				Msg("📨 EXTERNAL_AGENT_DEBUG: Received WebSocket message from control plane")

			// process message in a goroutine, if max goroutines are reached
			// the call will block until a goroutine is available
			pool.Go(func() {
				log.Debug().
					Str("EXTERNAL_AGENT_DEBUG", "processing_message").
					Msg("🔄 EXTERNAL_AGENT_DEBUG: Starting to process message in goroutine")

				if err := r.processMessage(ctx, conn, message); err != nil {
					log.Error().
						Str("EXTERNAL_AGENT_DEBUG", "message_processing_error").
						Err(err).
						Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to process message")
					return
				}

				newOps := ops.Add(1)
				log.Debug().
					Str("EXTERNAL_AGENT_DEBUG", "message_processed").
					Uint64("total_operations", newOps).
					Msg("✅ EXTERNAL_AGENT_DEBUG: Message processed successfully")

				// cancel context if max tasks are reached
				// TEMPORARILY DISABLED: Keep runner alive for multiple requests
				if false && r.cfg.MaxTasks > 0 && newOps >= uint64(r.cfg.MaxTasks) {
					log.Info().
						Str("EXTERNAL_AGENT_DEBUG", "max_tasks_reached").
						Int("max_tasks", r.cfg.MaxTasks).
						Uint64("current_operations", newOps).
						Msg("🛑 EXTERNAL_AGENT_DEBUG: Max tasks reached, cancelling context")
					cancel()
				}
			})

		}
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		// ping every 5 seconds to keep the connection alive
		case <-ticker.C:
			log.Info().
				Str("EXTERNAL_AGENT_DEBUG", "sending_ping").
				Str("runner_id", r.cfg.RunnerID).
				Msg("🏓 EXTERNAL_AGENT_DEBUG: Sending ping to control plane")

			err := conn.WriteMessage(websocket.PingMessage, []byte{})
			if err != nil {
				if strings.Contains(err.Error(), "broken pipe") {
					log.Error().
						Str("EXTERNAL_AGENT_DEBUG", "ping_broken_pipe").
						Str("runner_id", r.cfg.RunnerID).
						Err(err).
						Msg("❌ EXTERNAL_AGENT_DEBUG: Broken pipe when sending ping - control plane closed connection")
					return fmt.Errorf("Helix control-plane has closed connection, restarting (%s)", err)
				}

				log.Error().
					Str("EXTERNAL_AGENT_DEBUG", "ping_send_error").
					Str("runner_id", r.cfg.RunnerID).
					Err(err).
					Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to send ping message, closing connection")
				return fmt.Errorf("failed to write ping message (%w), closing connection", err)
			}

			log.Info().
				Str("EXTERNAL_AGENT_DEBUG", "ping_sent_successfully").
				Str("runner_id", r.cfg.RunnerID).
				Msg("🏓 EXTERNAL_AGENT_DEBUG: Ping sent successfully to control plane")
		}
	}
}

func (r *ExternalAgentRunner) dial(ctx context.Context) (*websocket.Conn, error) {
	var apiHost string

	if strings.HasPrefix(r.cfg.APIHost, "https://") {
		apiHost = strings.Replace(r.cfg.APIHost, "https", "wss", 1)
	}
	if strings.HasPrefix(r.cfg.APIHost, "http://") {
		apiHost = strings.Replace(r.cfg.APIHost, "http", "ws", 1)
	}

	// Use external-agent-runner endpoint (matching GPTScript pattern)
	apiHost = fmt.Sprintf("%s%s?access_token=%s&concurrency=%d&runnerid=%s",
		apiHost,
		system.GetAPIPath("/ws/external-agent-runner"),
		url.QueryEscape(r.cfg.APIToken), // Runner auth token to connect to the control plane
		r.cfg.Concurrency,               // Concurrency is the number of tasks the runner can handle concurrently
		r.cfg.RunnerID,                  // Runner ID is a unique identifier for the runner
	)

	// NOTE(milosgajdos): disabling bodyclose here as there is no need for closing the response
	// See: https://pkg.go.dev/github.com/gorilla/websocket@v1.5.3#Dialer.DialContext
	// nolint:bodyclose
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, apiHost, nil)
	if err != nil {
		log.Error().
			Str("EXTERNAL_AGENT_DEBUG", "websocket_dial_failed").
			Str("api_host", apiHost).
			Err(err).
			Msgf("❌ EXTERNAL_AGENT_DEBUG: WebSocket dial to '%s' failed, error: %s", apiHost, err)
		return nil, fmt.Errorf("websocket dial to '%s' failed, error: %s", apiHost, err)
	}

	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "connected_to_control_plane").
		Str("api_host", apiHost).
		Msg("🟢 EXTERNAL_AGENT_DEBUG: Connected to control plane successfully")

	return conn, nil
}

func (r *ExternalAgentRunner) processMessage(ctx context.Context, conn *websocket.Conn, message []byte) error {
	log.Debug().
		Str("EXTERNAL_AGENT_DEBUG", "unmarshalling_message").
		Int("message_length", len(message)).
		Msg("📋 EXTERNAL_AGENT_DEBUG: Unmarshalling message envelope")

	var envelope types.RunnerEventRequestEnvelope
	if err := json.Unmarshal(message, &envelope); err != nil {
		log.Error().
			Str("EXTERNAL_AGENT_DEBUG", "unmarshal_error").
			Err(err).
			Str("raw_message", string(message)).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to unmarshal message")
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "message_envelope_parsed").
		Str("request_id", envelope.RequestID).
		Str("reply", envelope.Reply).
		Str("type", string(envelope.Type)).
		Int("payload_length", len(envelope.Payload)).
		Msg("📦 EXTERNAL_AGENT_DEBUG: Message envelope parsed successfully")

	switch envelope.Type {
	case types.RunnerEventRequestZedAgent:
		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "processing_zed_agent_request").
			Str("request_id", envelope.RequestID).
			Msg("🎯 EXTERNAL_AGENT_DEBUG: Processing Zed agent request")
		return r.processZedAgentRequest(ctx, conn, &envelope)
	case types.RunnerEventRequestRDPData:
		log.Debug().
			Str("EXTERNAL_AGENT_DEBUG", "processing_rdp_data").
			Str("request_id", envelope.RequestID).
			Msg("🖥️ EXTERNAL_AGENT_DEBUG: Processing RDP data")
		return r.processRDPDataRequest(ctx, conn, &envelope)
	default:
		log.Error().
			Str("EXTERNAL_AGENT_DEBUG", "unknown_message_type").
			Str("type", string(envelope.Type)).
			Str("request_id", envelope.RequestID).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Unknown message type")
		return fmt.Errorf("unknown message type: %s", envelope.Type)
	}
}

func (r *ExternalAgentRunner) processZedAgentRequest(ctx context.Context, conn *websocket.Conn, req *types.RunnerEventRequestEnvelope) error {
	logger := log.With().
		Str("EXTERNAL_AGENT_DEBUG", "zed_agent_request").
		Str("request_id", req.RequestID).
		Logger()

	logger.Debug().
		Str("reply", req.Reply).
		Int("payload_length", len(req.Payload)).
		Msg("🔄 EXTERNAL_AGENT_DEBUG: Processing Zed agent request")

	var taskMessage types.ZedTaskMessage
	if err := json.Unmarshal(req.Payload, &taskMessage); err != nil {
		logger.Error().
			Str("EXTERNAL_AGENT_DEBUG", "task_message_unmarshal_error").
			Err(err).
			Str("payload", string(req.Payload)).
			Msgf("❌ EXTERNAL_AGENT_DEBUG: Failed to unmarshal task message (%s)", string(req.Payload))
		return fmt.Errorf("failed to unmarshal task message (%s): %w", string(req.Payload), err)
	}

	// Extract the agent from the task message
	agent := taskMessage.Agent

	logger.Info().
		Str("EXTERNAL_AGENT_DEBUG", "zed_agent_request_details").
		Str("session_id", agent.SessionID).
		Str("input", agent.Input).
		Str("project_path", agent.ProjectPath).
		Str("work_dir", agent.WorkDir).
		Msg("🚀 EXTERNAL_AGENT_DEBUG: Starting Zed agent")

	start := time.Now()

	// Start Zed agent in container with RDP
	resp, err := r.startZedAgent(ctx, &agent)
	if err != nil {
		logger.Error().
			Str("EXTERNAL_AGENT_DEBUG", "zed_agent_start_error").
			Err(err).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to start Zed agent")
		resp = &types.ZedAgentResponse{
			SessionID: agent.SessionID,
			Error:     err.Error(),
			Status:    "error",
		}
	} else {
		logger.Info().
			Str("EXTERNAL_AGENT_DEBUG", "zed_agent_started").
			Str("session_id", agent.SessionID).
			Str("status", resp.Status).
			Msg("✅ EXTERNAL_AGENT_DEBUG: Zed agent started successfully")
	}

	logger.Info().
		Str("EXTERNAL_AGENT_DEBUG", "zed_agent_request_completed").
		TimeDiff("duration", time.Now(), start).
		Msg("⏱️ EXTERNAL_AGENT_DEBUG: Zed agent request processed")

	return r.respond(conn, req.RequestID, req.Reply, resp)
}

// startZedAgent starts a Zed agent in a container with RDP access
func (r *ExternalAgentRunner) startZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	logger := log.With().Str("session_id", agent.SessionID).Logger()

	// Set up workspace directory
	workspaceDir := agent.WorkDir
	if workspaceDir == "" {
		workspaceDir = fmt.Sprintf("/workspace/%s", agent.SessionID)
	}

	// Set up project path
	projectPath := agent.ProjectPath
	if projectPath == "" {
		projectPath = workspaceDir
	}

	// Use RDP password provided by control plane (control plane rotates passwords)
	rdpPassword := agent.RDPPassword
	if rdpPassword == "" {
		logger.Error().Msg("No RDP password provided by control plane - failing securely")
		return nil, fmt.Errorf("RDP password not provided by control plane")
	}
	logger.Info().
		Str("session_id", agent.SessionID).
		Msg("Using RDP password provided by control plane (password rotated per session)")
	rdpPort := 5900 // Fixed RDP port inside container
	if agent.RDPPort != 0 {
		rdpPort = agent.RDPPort
	}

	logger.Info().
		Str("workspace_dir", workspaceDir).
		Str("project_path", projectPath).
		Int("rdp_port", rdpPort).
		Bool("password_from_control_plane", true).
		Msg("initializing Zed agent environment with RDP configuration")

	// Configure RDP server with the generated password
	err := r.configureRDPServer(ctx, rdpPassword, agent.SessionID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to configure RDP server")
		return nil, fmt.Errorf("failed to configure RDP server: %w", err)
	}

	// Actually start Zed binary with the workspace
	err = r.startZedBinary(ctx, workspaceDir, projectPath, agent)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to start Zed binary")
		return nil, fmt.Errorf("failed to start Zed binary: %w", err)
	}

	logger.Info().Msg("Zed binary started successfully")

	// Return successful response with runner ID (control plane already has RDP password)
	response := &types.ZedAgentResponse{
		SessionID: agent.SessionID,
		RunnerID:  r.cfg.RunnerID,
		Status:    "running",
		RDPURL:    fmt.Sprintf("rdp://localhost:%d", rdpPort),
	}

	logger.Info().
		Str("status", response.Status).
		Str("rdp_url", response.RDPURL).
		Msg("Zed agent started successfully")

	return response, nil
}

// generatePassword generates a secure password for RDP access
// startZedBinary spawns the actual Zed editor binary
func (r *ExternalAgentRunner) startZedBinary(ctx context.Context, workspaceDir, projectPath string, agent *types.ZedAgent) error {
	logger := log.With().Str("session_id", agent.SessionID).Logger()

	// Ensure workspace directory exists
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Set up environment for Zed
	env := []string{
		"DISPLAY=:1",
		"HOME=/home/zed",
		"USER=zed",
		"ZED_ALLOW_ROOT=true",
		fmt.Sprintf("WORKSPACE_DIR=%s", workspaceDir),
		fmt.Sprintf("PROJECT_PATH=%s", projectPath),
		fmt.Sprintf("HELIX_SESSION_ID=%s", agent.SessionID),
	}

	// Add any custom environment variables from agent config
	if agent.Env != nil {
		env = append(env, agent.Env...)
	}

	// Prepare Zed command - open the project path
	args := []string{"zed", projectPath}

	logger.Info().
		Str("workspace_dir", workspaceDir).
		Str("project_path", projectPath).
		Strs("args", args).
		Msg("Starting Zed binary")

	// Start Zed as a background process
	cmd := exec.CommandContext(ctx, "/usr/local/bin/zed", args[1:]...)
	cmd.Env = env
	cmd.Dir = workspaceDir

	// Set up process group to ensure proper cleanup
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Zed process: %w", err)
	}

	logger.Info().
		Int("pid", cmd.Process.Pid).
		Msg("Zed process started successfully")

	// Start a goroutine to monitor the process
	go func() {
		if err := cmd.Wait(); err != nil {
			logger.Error().Err(err).Msg("Zed process exited with error")
		} else {
			logger.Info().Msg("Zed process exited normally")
		}
	}()

	return nil
}

func (r *ExternalAgentRunner) generatePassword() string {
	// Generate a cryptographically secure random password for RDP access
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 16

	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		// Fail securely - no fallback passwords
		log.Error().Err(err).Msg("Failed to generate secure random password - cannot proceed")
		panic(fmt.Sprintf("Failed to generate secure RDP password: %v", err))
	}

	// Convert random bytes to charset characters
	password := make([]byte, length)
	for i := 0; i < length; i++ {
		password[i] = charset[b[i]%byte(len(charset))]
	}

	return fmt.Sprintf("zed_%s_%d", string(password), time.Now().Unix())
}

// configureRDPServer configures the actual RDP server with the generated password
func (r *ExternalAgentRunner) configureRDPServer(ctx context.Context, password, sessionID string) error {
	logger := log.With().
		Str("session_id", sessionID).
		Logger()

	logger.Info().Msg("Configuring RDP server with generated password")

	// Set the RDP password for the zed user using chpasswd
	cmd := exec.CommandContext(ctx, "chpasswd")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("zed:%s", password))

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error().
			Err(err).
			Str("output", string(output)).
			Msg("Failed to set RDP password with chpasswd")
		return fmt.Errorf("failed to set RDP password: %w", err)
	}

	logger.Info().Msg("RDP password configured successfully")

	// Verify the user account is properly configured
	cmd = exec.CommandContext(ctx, "id", "zed")
	output, err = cmd.CombinedOutput()
	if err != nil {
		logger.Warn().
			Err(err).
			Str("output", string(output)).
			Msg("Failed to verify zed user account")
		// Don't fail here, just log the warning
	} else {
		logger.Info().
			Str("user_info", string(output)).
			Msg("Verified zed user account exists")
	}

	return nil
}

// processRDPDataRequest handles RDP data forwarding to the local RDP server
func (r *ExternalAgentRunner) processRDPDataRequest(ctx context.Context, conn *websocket.Conn, req *types.RunnerEventRequestEnvelope) error {
	logger := log.With().
		Str("EXTERNAL_AGENT_DEBUG", "rdp_data_request").
		Str("request_id", req.RequestID).
		Logger()

	logger.Debug().
		Str("reply", req.Reply).
		Int("payload_length", len(req.Payload)).
		Msg("🖥️ EXTERNAL_AGENT_DEBUG: Processing RDP data request")

	// Unmarshal RDP data payload
	var rdpData types.ZedAgentRDPData
	if err := json.Unmarshal(req.Payload, &rdpData); err != nil {
		logger.Error().
			Str("EXTERNAL_AGENT_DEBUG", "rdp_unmarshal_error").
			Err(err).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to unmarshal RDP data")
		return r.respond(conn, req.RequestID, req.Reply, map[string]interface{}{
			"error": fmt.Sprintf("failed to unmarshal RDP data: %v", err),
		})
	}

	logger.Debug().
		Str("rdp_type", rdpData.Type).
		Int("data_length", len(rdpData.Data)).
		Msg("🖥️ EXTERNAL_AGENT_DEBUG: RDP data unmarshalled")

	// Forward RDP data to local XRDP server
	err := r.forwardRDPDataToXRDP(ctx, conn, &rdpData, req.Reply)
	if err != nil {
		logger.Error().
			Err(err).
			Str("session_id", rdpData.SessionID).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to forward RDP data to XRDP")
		return r.respond(conn, req.RequestID, req.Reply, map[string]interface{}{
			"error": fmt.Sprintf("failed to forward RDP data: %v", err),
		})
	}

	logger.Debug().
		Str("session_id", rdpData.SessionID).
		Int("data_length", len(rdpData.Data)).
		Msg("✅ EXTERNAL_AGENT_DEBUG: RDP data forwarded to XRDP")

	// Don't respond immediately - response will come from XRDP data flow
	return nil
}

// forwardRDPDataToXRDP forwards RDP data to the local XRDP server and sets up response handling
func (r *ExternalAgentRunner) forwardRDPDataToXRDP(ctx context.Context, wsConn *websocket.Conn, rdpData *types.ZedAgentRDPData, replyTopic string) error {
	// Determine connection type and key
	// If SessionID matches RunnerID, it's a runner connection; otherwise it's a session connection
	connectionKey := rdpData.SessionID
	var connectionType string
	if connectionKey == r.cfg.RunnerID {
		connectionType = "runner"
	} else {
		connectionType = "session"
	}

	logger := log.With().
		Str("connection_key", connectionKey).
		Str("connection_type", connectionType).
		Logger()

	r.rdpMutex.Lock()
	rdpConn, exists := r.rdpConnections[connectionKey]
	r.rdpMutex.Unlock()

	if !exists {
		// Create new connection to local XRDP server
		var err error
		rdpConn, err = r.createRDPConnection(ctx, wsConn, connectionKey, connectionType, replyTopic)
		if err != nil {
			return fmt.Errorf("failed to create RDP connection: %w", err)
		}

		logger.Info().
			Str("connection_type", connectionType).
			Msg("Created new RDP connection to local XRDP server")
	}

	// Forward the RDP data to XRDP
	_, err := rdpConn.conn.Write(rdpData.Data)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to write RDP data to XRDP server")

		// Clean up failed connection
		r.cleanupRDPConnection(connectionKey)
		return fmt.Errorf("failed to write to XRDP: %w", err)
	}

	logger.Debug().
		Int("bytes_written", len(rdpData.Data)).
		Msg("RDP data written to XRDP server")

	return nil
}

// createRDPConnection establishes a connection to the local XRDP server
func (r *ExternalAgentRunner) createRDPConnection(ctx context.Context, wsConn *websocket.Conn, connectionKey, connectionType, replyTopic string) (*RDPConnection, error) {
	logger := log.With().
		Str("connection_key", connectionKey).
		Str("connection_type", connectionType).
		Logger()

	// Connect to local XRDP server (typically on port 3389)
	rdpAddr := fmt.Sprintf("localhost:%d", r.cfg.RDPStartPort)
	conn, err := net.Dial("tcp", rdpAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to XRDP at %s: %w", rdpAddr, err)
	}

	logger.Info().
		Str("xrdp_addr", rdpAddr).
		Str("connection_type", connectionType).
		Msg("Connected to local XRDP server")

	// Create RDP connection context
	rdpCtx, cancel := context.WithCancel(ctx)

	rdpConnection := &RDPConnection{
		connectionKey:  connectionKey,
		connectionType: connectionType,
		runnerID:       r.cfg.RunnerID,
		conn:           conn,
		replyTopic:     replyTopic,
		websocketConn:  wsConn,
		ctx:            rdpCtx,
		cancel:         cancel,
	}

	// Store the connection
	r.rdpMutex.Lock()
	r.rdpConnections[connectionKey] = rdpConnection
	r.rdpMutex.Unlock()

	// Start goroutine to handle responses from XRDP back to control plane
	go r.handleRDPResponses(rdpConnection)

	return rdpConnection, nil
}

// handleRDPResponses handles data coming back from XRDP server and forwards it via NATS
func (r *ExternalAgentRunner) handleRDPResponses(rdpConn *RDPConnection) {
	logger := log.With().
		Str("connection_key", rdpConn.connectionKey).
		Str("connection_type", rdpConn.connectionType).
		Logger()

	defer func() {
		rdpConn.cancel()
		rdpConn.conn.Close()
		r.cleanupRDPConnection(rdpConn.connectionKey)
		logger.Info().Msg("RDP response handler cleaned up")
	}()

	buffer := make([]byte, 4096)

	for {
		select {
		case <-rdpConn.ctx.Done():
			logger.Debug().Msg("RDP response handler context cancelled")
			return
		default:
		}

		// Set read timeout to allow context cancellation
		rdpConn.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := rdpConn.conn.Read(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout, check context and continue
				continue
			}
			logger.Debug().Err(err).Msg("RDP connection closed by XRDP server")
			return
		}

		// Forward response data back to control plane via WebSocket response
		responseData := types.ZedAgentRDPData{
			SessionID: rdpConn.connectionKey, // Use the original connection key (could be sessionID or runnerID)
			Type:      "rdp_response",
			Data:      buffer[:n],
			Timestamp: time.Now().Unix(),
		}

		// Send response back to control plane via NATS reply topic
		responsePayload, err := json.Marshal(responseData)
		if err != nil {
			logger.Error().
				Err(err).
				Msg("Failed to marshal RDP response data")
			continue
		}

		// Send the response directly via WebSocket as a reply message
		replyMessage := types.RunnerEventResponseEnvelope{
			RequestID: fmt.Sprintf("rdp-response-%d", time.Now().UnixNano()),
			Reply:     rdpConn.replyTopic,
			Payload:   responsePayload,
		}

		err = rdpConn.websocketConn.WriteJSON(replyMessage)
		if err != nil {
			logger.Error().
				Err(err).
				Msg("Failed to send RDP response back to control plane")
			return
		}

		logger.Debug().
			Int("bytes_sent", n).
			Msg("RDP response data sent back to control plane")
	}
}

// cleanupRDPConnection removes and cleans up an RDP connection
func (r *ExternalAgentRunner) cleanupRDPConnection(connectionKey string) {
	r.rdpMutex.Lock()
	defer r.rdpMutex.Unlock()

	if rdpConn, exists := r.rdpConnections[connectionKey]; exists {
		rdpConn.cancel()
		rdpConn.conn.Close()
		delete(r.rdpConnections, connectionKey)

		log.Info().
			Str("connection_key", connectionKey).
			Msg("Cleaned up RDP connection")
	}
}

// cleanupAllRDPConnections cleans up all RDP connections when runner stops
func (r *ExternalAgentRunner) cleanupAllRDPConnections() {
	r.rdpMutex.Lock()
	defer r.rdpMutex.Unlock()

	log.Info().
		Int("connection_count", len(r.rdpConnections)).
		Msg("Cleaning up all RDP connections")

	for connectionKey, rdpConn := range r.rdpConnections {
		rdpConn.cancel()
		rdpConn.conn.Close()
		log.Debug().
			Str("connection_key", connectionKey).
			Msg("Closed RDP connection")
	}

	// Clear the map
	r.rdpConnections = make(map[string]*RDPConnection)

	log.Info().Msg("All RDP connections cleaned up")
}

func (r *ExternalAgentRunner) respond(conn *websocket.Conn, reqID, reply string, resp interface{}) error {
	log.Debug().
		Str("EXTERNAL_AGENT_DEBUG", "marshalling_response").
		Str("request_id", reqID).
		Str("reply", reply).
		Msg("📤 EXTERNAL_AGENT_DEBUG: Marshalling response")

	bts, err := json.Marshal(resp)
	if err != nil {
		log.Error().
			Str("EXTERNAL_AGENT_DEBUG", "response_marshal_error").
			Str("request_id", reqID).
			Err(err).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to marshal response")
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	env := types.RunnerEventResponseEnvelope{
		RequestID: reqID,
		Reply:     reply,
		Payload:   bts,
	}

	bts, err = json.Marshal(env)
	if err != nil {
		log.Error().
			Str("EXTERNAL_AGENT_DEBUG", "envelope_marshal_error").
			Str("request_id", reqID).
			Err(err).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to marshal external agent response envelope")
		return fmt.Errorf("failed to marshal external agent response envelope: %w", err)
	}

	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "sending_response").
		Str("request_id", reqID).
		Str("reply", reply).
		Int("response_length", len(bts)).
		Msg("📡 EXTERNAL_AGENT_DEBUG: Sending response to control plane")

	if err := conn.WriteMessage(websocket.TextMessage, bts); err != nil {
		log.Error().
			Str("EXTERNAL_AGENT_DEBUG", "websocket_write_error").
			Str("request_id", reqID).
			Err(err).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to write message to WebSocket")
		return fmt.Errorf("failed to write message: %w", err)
	}

	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "response_sent").
		Str("request_id", reqID).
		Str("reply", reply).
		Msg("✅ EXTERNAL_AGENT_DEBUG: Response sent successfully")

	return nil
}
