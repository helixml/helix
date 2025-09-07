package external_agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	cfg *config.ExternalAgentRunnerConfig
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
			err := conn.WriteMessage(websocket.PingMessage, []byte{})
			if err != nil {
				if strings.Contains(err.Error(), "broken pipe") {
					return fmt.Errorf("Helix control-plane has closed connection, restarting (%s)", err)
				}

				log.Err(err).Msg("failed to write ping message, closing connection")
				return fmt.Errorf("failed to write ping message (%w), closing connection", err)
			}
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

	var agent types.ZedAgent
	if err := json.Unmarshal(req.Payload, &agent); err != nil {
		logger.Error().
			Str("EXTERNAL_AGENT_DEBUG", "zed_agent_unmarshal_error").
			Err(err).
			Str("payload", string(req.Payload)).
			Msgf("❌ EXTERNAL_AGENT_DEBUG: Failed to unmarshal Zed agent (%s)", string(req.Payload))
		return fmt.Errorf("failed to unmarshal Zed agent (%s): %w", string(req.Payload), err)
	}

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
	// Generate RDP credentials
	rdpPassword := r.generatePassword()
	rdpPort := 5900 // Fixed RDP port inside container

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

	logger.Info().
		Str("workspace_dir", workspaceDir).
		Str("project_path", projectPath).
		Int("rdp_port", rdpPort).
		Msg("initializing Zed agent environment")

	// Actually start Zed binary with the workspace
	err := r.startZedBinary(ctx, workspaceDir, projectPath, agent)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to start Zed binary")
		return nil, fmt.Errorf("failed to start Zed binary: %w", err)
	}

	logger.Info().Msg("Zed binary started successfully")

	// Return successful response with RDP info
	response := &types.ZedAgentResponse{
		SessionID:   agent.SessionID,
		Status:      "running",
		RDPURL:      fmt.Sprintf("rdp://localhost:%d", rdpPort),
		RDPPassword: rdpPassword,
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
	// Simple password generation - use crypto/rand in production
	return fmt.Sprintf("zed_%d", time.Now().Unix())
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
