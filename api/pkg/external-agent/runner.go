package external_agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
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
// and listens for external agent tasks to run
type ExternalAgentRunner struct {
	cfg *config.ExternalAgentRunnerConfig
}

func NewExternalAgentRunner(cfg *config.ExternalAgentRunnerConfig) *ExternalAgentRunner {
	return &ExternalAgentRunner{
		cfg: cfg,
	}
}

func (r *ExternalAgentRunner) Run(ctx context.Context) error {
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
		Int("concurrency", r.cfg.Concurrency).
		Int("max_tasks", r.cfg.MaxTasks).
		Msg("🟢 starting external agent task processing")

	go func() {
		defer close(done)
		for {
			mt, message, err := conn.ReadMessage()
			if err != nil {
				if errors.Is(ctx.Err(), context.Canceled) {
					return
				}
				log.Err(err).Msg("failed to read websocket message")
				return
			}

			if mt != websocket.TextMessage {
				continue
			}

			// process message in a goroutine, if max goroutines are reached
			// the call will block until a goroutine is available
			pool.Go(func() {
				if err := r.processMessage(ctx, conn, message); err != nil {
					log.Err(err).Msg("failed to process message")
					return
				}
				ops.Add(1)

				// cancel context if max tasks are reached
				if r.cfg.MaxTasks > 0 && ops.Load() >= uint64(r.cfg.MaxTasks) {
					log.Info().Msg("max tasks reached, cancelling context")
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

	// Use external-agent-runner endpoint
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
		log.Error().Msgf("websocket dial to '%s' failed, error: %s", apiHost, err)
		return nil, fmt.Errorf("websocket dial to '%s' failed, error: %s", apiHost, err)
	}

	log.Info().Msg("🟢 connected to control plane")

	return conn, nil
}

func (r *ExternalAgentRunner) processMessage(ctx context.Context, conn *websocket.Conn, message []byte) error {
	var envelope types.RunnerEventRequestEnvelope
	if err := json.Unmarshal(message, &envelope); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	switch envelope.Type {
	case types.RunnerEventRequestZedAgent:
		return r.processZedAgentRequest(ctx, conn, &envelope)
	default:
		return fmt.Errorf("unknown message type: %s", envelope.Type)
	}
}

func (r *ExternalAgentRunner) processZedAgentRequest(ctx context.Context, conn *websocket.Conn, req *types.RunnerEventRequestEnvelope) error {
	logger := log.With().Str("request_id", req.RequestID).Logger()

	var agent types.ZedAgent
	if err := json.Unmarshal(req.Payload, &agent); err != nil {
		logger.Err(err).Msgf("failed to unmarshal Zed agent (%s)", string(req.Payload))
		return fmt.Errorf("failed to unmarshal Zed agent (%s): %w", string(req.Payload), err)
	}

	logger.Debug().
		Str("session_id", agent.SessionID).
		Str("input", agent.Input).
		Str("project_path", agent.ProjectPath).
		Str("work_dir", agent.WorkDir).
		Msg("processing Zed agent request")

	start := time.Now()

	// ZedExecutor not available in this configuration
	err := fmt.Errorf("Zed execution not supported")
	logger.Error().Err(err).Msg("failed to start Zed agent")
	resp := &types.ZedAgentResponse{
		SessionID: agent.SessionID,
		Error:     err.Error(),
		Status:    "error",
	}

	logger.Info().TimeDiff("duration", time.Now(), start).Msg("Zed agent request processed")

	return r.respond(conn, req.RequestID, req.Reply, resp)
}

func (r *ExternalAgentRunner) respond(conn *websocket.Conn, reqID, reply string, resp interface{}) error {
	bts, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	env := types.RunnerEventResponseEnvelope{
		RequestID: reqID,
		Reply:     reply,
		Payload:   bts,
	}

	bts, err = json.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to marshal external agent response envelope: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, bts); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}
