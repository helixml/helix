package gptscript

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

// Runner connects using a WebSocket to the Control Plane
// and listens for GPTScript tasks to run
type Runner struct {
	cfg *config.GPTScriptRunnerConfig
}

func NewRunner(cfg *config.GPTScriptRunnerConfig) *Runner {
	return &Runner{
		cfg: cfg,
	}
}

func (d *Runner) Run(ctx context.Context) error {
	// TODO: retry loop?
	return d.run(ctx)
}

func (d *Runner) run(ctx context.Context) error {
	var conn *websocket.Conn

	err := retry.Do(func() error {
		var err error
		conn, err = d.dial(ctx)
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

	pool := pool.New().WithMaxGoroutines(d.cfg.Concurrency)
	var ops atomic.Uint64

	ctx, cancel := context.WithCancel(ctx)

	log.Info().
		Int("concurrency", d.cfg.Concurrency).
		Int("max_tasks", d.cfg.MaxTasks).
		Msg("🟢 starting task processing")

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
				if err := d.processMessage(ctx, conn, message); err != nil {
					log.Err(err).Msg("failed to process message")
					return
				}
				ops.Add(1)

				// cancel context if max tasks are reached
				if d.cfg.MaxTasks > 0 && ops.Load() >= uint64(d.cfg.MaxTasks) {
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
		// ping every 10 seconds to keep the connection alive
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

func (d *Runner) dial(ctx context.Context) (*websocket.Conn, error) {
	var apiHost string

	if strings.HasPrefix(d.cfg.APIHost, "https://") {
		apiHost = strings.Replace(d.cfg.APIHost, "https", "wss", 1)
	}
	if strings.HasPrefix(d.cfg.APIHost, "http://") {
		apiHost = strings.Replace(d.cfg.APIHost, "http", "ws", 1)
	}

	apiHost = fmt.Sprintf("%s%s?access_token=%s&concurrency=%d&runnerid=%s",
		apiHost,
		system.GetApiPath("/ws/gptscript-runner"),
		url.QueryEscape(d.cfg.APIToken), // Runner auth token to connect to the control plane
		d.cfg.Concurrency,               // Concurrency is the number of tasks the runner can handle concurrently
		d.cfg.RunnerID,                  // Runner ID is a unique identifier for the runner
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

func (d *Runner) processMessage(ctx context.Context, conn *websocket.Conn, message []byte) error {
	var envelope types.RunnerEventRequestEnvelope
	if err := json.Unmarshal(message, &envelope); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	switch envelope.Type {
	case types.RunnerEventRequestApp:
		return d.processAppRequest(ctx, conn, &envelope)
	case types.RunnerEventRequestTool:
		return d.processToolRequest(ctx, conn, &envelope)
	default:
		return fmt.Errorf("unknown message type: %s", envelope.Type)
	}
}

func (d *Runner) processAppRequest(ctx context.Context, conn *websocket.Conn, req *types.RunnerEventRequestEnvelope) error {
	logger := log.With().Str("request_id", req.RequestID).Logger()

	var app types.GptScriptGithubApp
	if err := json.Unmarshal(req.Payload, &app); err != nil {
		logger.Err(err).Msgf("failed to unmarshal GPTScript app (%s)", string(req.Payload))
		return fmt.Errorf("failed to unmarshal GPTScript app (%s): %w", string(req.Payload), err)
	}

	logger.Debug().
		Str("repo", app.Repo).
		Str("script_input", app.Script.Input).
		Msg("processing GPTScript app request")

	start := time.Now()

	resp, err := RunGPTAppScript(ctx, &app)
	if err != nil {
		return fmt.Errorf("failed to run GPTScript app: %w", err)
	}

	logger.Info().TimeDiff("duration", time.Now(), start).Msg("message processed")

	return d.respond(conn, req.RequestID, req.Reply, resp)
}

func (d *Runner) processToolRequest(ctx context.Context, conn *websocket.Conn, req *types.RunnerEventRequestEnvelope) error {
	logger := log.With().Str("request_id", req.RequestID).Logger()

	var script types.GptScript
	if err := json.Unmarshal(req.Payload, &script); err != nil {
		return fmt.Errorf("failed to unmarshal GPTScript tool (%s): %w", string(req.Payload), err)
	}

	logger.Debug().
		Str("script_input", script.Input).
		Str("file_path", script.FilePath).
		Str("source", script.Source).
		Msg("processing GPTScript tool request")

	start := time.Now()

	resp, err := RunGPTScript(ctx, &script)
	if err != nil {
		return fmt.Errorf("failed to run GPTScript tool: %w", err)
	}

	logger.Info().TimeDiff("duration", time.Now(), start).Msg("message processed")

	return d.respond(conn, req.RequestID, req.Reply, resp)
}

func (r *Runner) respond(conn *websocket.Conn, reqID, reply string, resp interface{}) error {
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
		return fmt.Errorf("failed to marshal GPTScript tool response envelope: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, bts); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}
