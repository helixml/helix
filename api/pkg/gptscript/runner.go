package gptscript

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	retries             = 5
	delayBetweenRetries = time.Second
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
	)
	if err != nil {
		return err
	}

	defer conn.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			mt, message, err := conn.ReadMessage()
			if err != nil {
				log.Err(err).Msg("failed to read websocket message")
				return
			}

			if mt != websocket.TextMessage {
				continue
			}

			if err := d.processMessage(ctx, conn, message); err != nil {
				log.Err(err).Msg("failed to process message")
				return
			}
			log.Info().Msg("message processed")
		}
	}()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		// ping every 10 seconds to keep the connection alive
		case <-ticker.C:
			err := conn.WriteMessage(websocket.PingMessage, []byte{})
			if err != nil {
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

	apiHost = fmt.Sprintf("%s%s?access_token=%s", apiHost, system.GetApiPath("/ws/gptscript-runner"), d.cfg.APIToken)

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
	var app types.GptScriptGithubApp
	if err := json.Unmarshal(req.Payload, &app); err != nil {
		return fmt.Errorf("failed to unmarshal GPTScript app (%s): %w", string(req.Payload), err)
	}

	log.Info().Str("repo", app.Repo).Msg("processing GPTScript app request")

	resp, err := RunGPTAppScript(ctx, &app)
	if err != nil {
		return fmt.Errorf("failed to run GPTScript app: %w", err)
	}

	return d.respond(conn, req.Reply, resp)
}

func (d *Runner) processToolRequest(ctx context.Context, conn *websocket.Conn, req *types.RunnerEventRequestEnvelope) error {
	var script types.GptScript
	if err := json.Unmarshal(req.Payload, &script); err != nil {
		return fmt.Errorf("failed to unmarshal GPTScript tool (%s): %w", string(req.Payload), err)
	}

	log.Info().Str("script", script.Input).Msg("processing GPTScript tool request")

	resp, err := RunGPTScript(ctx, &script)
	if err != nil {
		return fmt.Errorf("failed to run GPTScript tool: %w", err)
	}

	return d.respond(conn, req.Reply, resp)
}

func (r *Runner) respond(conn *websocket.Conn, reply string, resp interface{}) error {
	bts, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	env := types.RunnerEventResponseEnvelope{
		Reply:   reply,
		Payload: bts,
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