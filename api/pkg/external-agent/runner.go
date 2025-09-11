package external_agent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/revdial"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	retries             = 100
	delayBetweenRetries = 3 * time.Second
)

// ExternalAgentRunner connects using a WebSocket to the Control Plane
// and listens for external agent tasks to run (follows GPTScript runner pattern)
// Also establishes reverse dial connection for RDP proxy
type ExternalAgentRunner struct {
	cfg      *config.ExternalAgentRunnerConfig
	revDialConn net.Conn // Reverse dial connection to control plane for RDP
}

func NewExternalAgentRunner(cfg *config.ExternalAgentRunnerConfig) *ExternalAgentRunner {
	return &ExternalAgentRunner{
		cfg: cfg,
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
		Msg("🚀 EXTERNAL_AGENT_DEBUG: Starting external agent runner with auto-reconnect")
	
	// Persistent retry loop - keep trying to reconnect on failures
	for {
		select {
		case <-ctx.Done():
			log.Info().
				Str("EXTERNAL_AGENT_DEBUG", "runner_shutdown").
				Str("runner_id", r.cfg.RunnerID).
				Msg("🛑 EXTERNAL_AGENT_DEBUG: Runner shutting down due to context cancellation")
			return ctx.Err()
		default:
		}
		
		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "connection_attempt").
			Str("runner_id", r.cfg.RunnerID).
			Msg("🔄 EXTERNAL_AGENT_DEBUG: Attempting to connect to control plane")
		
		err := r.run(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Info().
					Str("EXTERNAL_AGENT_DEBUG", "runner_cancelled").
					Str("runner_id", r.cfg.RunnerID).
					Msg("🛑 EXTERNAL_AGENT_DEBUG: Runner cancelled, exiting")
				return err
			}
			
			log.Error().
				Err(err).
				Str("EXTERNAL_AGENT_DEBUG", "connection_failed").
				Str("runner_id", r.cfg.RunnerID).
				Msg("❌ EXTERNAL_AGENT_DEBUG: Connection failed, will retry in 5 seconds")
			
			// Wait before retrying
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				// Continue to next iteration
			}
		}
	}
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

	// Establish reverse dial connection for RDP proxy
	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "establishing_reverse_dial").
		Str("runner_id", r.cfg.RunnerID).
		Msg("🔗 EXTERNAL_AGENT_DEBUG: Establishing reverse dial connection for RDP proxy")
	
	err = r.establishReverseDialConnection(ctx)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", r.cfg.RunnerID).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to establish reverse dial connection, will retry on next reconnect")
		return fmt.Errorf("failed to establish reverse dial connection: %w", err)
	}
	
	defer func() {
		if r.revDialConn != nil {
			r.revDialConn.Close()
		}
	}()

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
					Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to read websocket message - connection will be retried")
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
						Msg("❌ EXTERNAL_AGENT_DEBUG: Broken pipe when sending ping - control plane closed connection, will reconnect")
					return fmt.Errorf("Helix control-plane has closed connection, will auto-reconnect (%s)", err)
				}

				log.Error().
					Str("EXTERNAL_AGENT_DEBUG", "ping_send_error").
					Str("runner_id", r.cfg.RunnerID).
					Err(err).
					Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to send ping message, will reconnect")
				return fmt.Errorf("failed to write ping message (%w), will auto-reconnect", err)
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
		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "rdp_data_ignored").
			Str("request_id", envelope.RequestID).
			Msg("🖥️ EXTERNAL_AGENT_DEBUG: RDP data request ignored - using reverse dial instead")
		// RDP is now handled via reverse dial, not via WebSocket messages
		return nil
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

	// Skip RDP password rotation for testing - use static password from container
	logger.Info().
		Str("session_id", agent.SessionID).
		Msg("Skipping RDP password rotation - using static password for testing")
	
	rdpPort := 5900 // Fixed RDP port inside container  
	if agent.RDPPort != 0 {
		rdpPort = agent.RDPPort
	}

	logger.Info().
		Str("workspace_dir", workspaceDir).
		Str("project_path", projectPath).
		Int("rdp_port", rdpPort).
		Bool("password_rotation_disabled", true).
		Msg("initializing Zed agent environment with static RDP password")

	// Actually start Zed binary with the workspace
	err := r.startZedBinary(ctx, workspaceDir, projectPath, agent)
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

// establishReverseDialConnection establishes reverse dial connection to control plane for RDP proxy
func (r *ExternalAgentRunner) establishReverseDialConnection(ctx context.Context) error {
	// Build reverse dial URL 
	var apiHost string
	if strings.HasPrefix(r.cfg.APIHost, "https://") {
		apiHost = strings.Replace(r.cfg.APIHost, "https", "http", 1)
	} else if strings.HasPrefix(r.cfg.APIHost, "http://") {
		apiHost = r.cfg.APIHost
	} else {
		apiHost = "http://" + r.cfg.APIHost
	}
	
	revDialURL := fmt.Sprintf("%s%s?runnerid=%s",
		apiHost,
		system.GetAPIPath("/revdial"),
		url.QueryEscape(r.cfg.RunnerID),
	)
	
	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "reverse_dial_connecting").
		Str("url", revDialURL).
		Str("runner_id", r.cfg.RunnerID).
		Msg("🔗 EXTERNAL_AGENT_DEBUG: Establishing reverse dial connection")
	
	// Create HTTP request with runner token authentication
	req, err := http.NewRequestWithContext(ctx, "GET", revDialURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create reverse dial request: %w", err)
	}
	
	// Add Authorization header with runner token
	req.Header.Set("Authorization", "Bearer "+r.cfg.APIToken)
	
	// Use a custom transport to get access to the underlying connection
	// We need to hijack the connection like the control plane does
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	
	// Parse URL to get host:port
	u, err := url.Parse(revDialURL)
	if err != nil {
		return fmt.Errorf("failed to parse reverse dial URL: %w", err)
	}
	
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "https" {
			host = host + ":443"
		} else {
			host = host + ":80"
		}
	}
	
	// Establish raw TCP connection
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return fmt.Errorf("failed to dial control plane: %w", err)
	}
	
	// Send HTTP request over the connection
	reqLine := fmt.Sprintf("GET %s HTTP/1.1\r\n", u.RequestURI())
	hostHeader := fmt.Sprintf("Host: %s\r\n", u.Host)
	authHeader := fmt.Sprintf("Authorization: Bearer %s\r\n", r.cfg.APIToken)
	httpRequest := reqLine + hostHeader + authHeader + "\r\n"
	
	if _, err := conn.Write([]byte(httpRequest)); err != nil {
		conn.Close()
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	
	// Read HTTP response to confirm connection was hijacked
	respBytes := make([]byte, 1024)
	n, err := conn.Read(respBytes)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to read HTTP response: %w", err)
	}
	
	response := string(respBytes[:n])
	if !strings.Contains(response, "200 OK") {
		conn.Close()
		return fmt.Errorf("reverse dial endpoint returned non-200 response: %s", response)
	}
	
	// Connection is now hijacked and registered in connman on control plane side
	r.revDialConn = conn
	
	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "reverse_dial_established").
		Str("runner_id", r.cfg.RunnerID).
		Msg("✅ EXTERNAL_AGENT_DEBUG: Reverse dial connection established")
	
	// Start handling reverse dial connections in background
	go r.handleReverseDialForwarding(ctx)
	
	return nil
}

// handleReverseDialForwarding handles the reverse dial connection and forwards to XRDP
func (r *ExternalAgentRunner) handleReverseDialForwarding(ctx context.Context) {
	defer r.revDialConn.Close()
	
	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "reverse_dial_forwarding_start").
		Str("runner_id", r.cfg.RunnerID).
		Msg("🖥️ EXTERNAL_AGENT_DEBUG: Starting revdial listener for VNC")
	
	// Create a dialServer function that creates WebSocket connections back to the control plane
	// This is used by the revdial protocol for establishing data connections
	dialServer := func(ctx context.Context, path string) (*websocket.Conn, *http.Response, error) {
		// Build WebSocket URL for reverse dial data connections
		wsURL := strings.Replace(r.cfg.APIHost, "http://", "ws://", 1) + path
		
		log.Debug().
			Str("EXTERNAL_AGENT_DEBUG", "websocket_dial").
			Str("url", wsURL).
			Str("runner_id", r.cfg.RunnerID).
			Msg("🔗 EXTERNAL_AGENT_DEBUG: Dialing WebSocket for revdial data connection")
		
		// Create WebSocket connection with proper headers
		headers := http.Header{}
		dialer := websocket.Dialer{}
		conn, resp, err := dialer.DialContext(ctx, wsURL, headers)
		if err != nil {
			log.Error().
				Err(err).
				Str("url", wsURL).
				Str("runner_id", r.cfg.RunnerID).
				Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to dial WebSocket for revdial")
			return nil, resp, err
		}
		
		log.Debug().
			Str("EXTERNAL_AGENT_DEBUG", "websocket_connected").
			Str("url", wsURL).
			Str("runner_id", r.cfg.RunnerID).
			Msg("✅ EXTERNAL_AGENT_DEBUG: WebSocket connected for revdial data connection")
		
		return conn, resp, nil
	}
	
	// Create a revdial listener from the established connection
	// This handles the JSON control protocol and creates logical connections via WebSocket
	listener := revdial.NewListener(r.revDialConn, dialServer)
	defer listener.Close()
	
	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "reverse_dial_listener_created").
		Str("runner_id", r.cfg.RunnerID).
		Msg("✅ EXTERNAL_AGENT_DEBUG: Created revdial listener")
	
	// Accept incoming logical connections and forward each to VNC
	for {
		select {
		case <-ctx.Done():
			log.Info().
				Str("EXTERNAL_AGENT_DEBUG", "reverse_dial_context_cancelled").
				Str("runner_id", r.cfg.RunnerID).
				Msg("🔄 EXTERNAL_AGENT_DEBUG: Context cancelled, stopping revdial forwarding")
			return
		default:
		}
		
		// Accept an incoming logical connection from the control plane
		incomingConn, err := listener.Accept()
		if err != nil {
			log.Error().
				Err(err).
				Str("EXTERNAL_AGENT_DEBUG", "reverse_dial_accept_failed").
				Str("runner_id", r.cfg.RunnerID).
				Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to accept revdial connection")
			return
		}
		
		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "reverse_dial_connection_accepted").
			Str("runner_id", r.cfg.RunnerID).
			Msg("🔗 EXTERNAL_AGENT_DEBUG: Accepted new revdial logical connection")
		
		// Handle this connection in a goroutine
		go r.handleSingleVNCForwarding(ctx, incomingConn)
	}
}

// handleSingleVNCForwarding handles a single connection by forwarding it to VNC
func (r *ExternalAgentRunner) handleSingleVNCForwarding(ctx context.Context, incomingConn net.Conn) {
	defer incomingConn.Close()
	
	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "vnc_forward_start").
		Str("runner_id", r.cfg.RunnerID).
		Msg("🖥️ EXTERNAL_AGENT_DEBUG: Starting VNC forwarding for single connection")
	
	// Connect to local VNC server
	vncConn, err := net.Dial("tcp", "localhost:5902")
	if err != nil {
		log.Error().
			Err(err).
			Str("EXTERNAL_AGENT_DEBUG", "vnc_dial_failed").
			Str("runner_id", r.cfg.RunnerID).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to connect to local VNC")
		return
	}
	defer vncConn.Close()
	
	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "vnc_connected").
		Str("runner_id", r.cfg.RunnerID).
		Msg("✅ EXTERNAL_AGENT_DEBUG: Connected to local VNC server")
	
	// Simple bidirectional TCP proxy (no protocol conversion)
	done := make(chan struct{}, 2)
	
	// Forward control plane → VNC
	go func() {
		defer func() { done <- struct{}{} }()
		bytes, err := io.Copy(vncConn, incomingConn)
		log.Debug().
			Err(err).
			Int64("bytes", bytes).
			Str("direction", "control_plane->vnc").
			Str("runner_id", r.cfg.RunnerID).
			Msg("🔄 EXTERNAL_AGENT_DEBUG: TCP forward completed/ended")
	}()
	
	// Forward VNC → control plane  
	go func() {
		defer func() { done <- struct{}{} }()
		bytes, err := io.Copy(incomingConn, vncConn)
		log.Debug().
			Err(err).
			Int64("bytes", bytes).
			Str("direction", "vnc->control_plane").
			Str("runner_id", r.cfg.RunnerID).
			Msg("🔄 EXTERNAL_AGENT_DEBUG: TCP forward completed/ended")
	}()
	
	// Wait for either direction to complete
	<-done
	
	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "vnc_forward_complete").
		Str("runner_id", r.cfg.RunnerID).
		Msg("🔄 EXTERNAL_AGENT_DEBUG: VNC forwarding completed for single connection")
}



