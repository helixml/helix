package gptscript

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
)

//go:generate mockgen -source $GOFILE -destination gptscript_mocks.go -package $GOPACKAGE

type Executor interface {
	ExecuteApp(ctx context.Context, app *types.GptScriptGithubApp) (*types.GptScriptResponse, error)
	ExecuteScript(ctx context.Context, script *types.GptScript) (*types.GptScriptResponse, error)
}

// DefaultExecutor runs GPTScript scripts on the GPTScript cluster through the
// Helix control-plane (runners need to be running and connected)
type DefaultExecutor struct {
	cfg    *config.ServerConfig
	pubsub pubsub.PubSub
}

var _ Executor = &DefaultExecutor{}

func NewExecutor(cfg *config.ServerConfig, pubsub pubsub.PubSub) *DefaultExecutor {
	return &DefaultExecutor{
		cfg:    cfg,
		pubsub: pubsub,
	}
}

func (e *DefaultExecutor) ExecuteApp(ctx context.Context, app *types.GptScriptGithubApp) (*types.GptScriptResponse, error) {
	bts, err := json.Marshal(app)
	if err != nil {
		return nil, err
	}

	var retries int

	resp, err := retry.DoWithData(func() ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		header := map[string]string{
			"kind": "app",
		}

		resp, err := e.pubsub.StreamRequest(ctx, pubsub.ScriptRunnerStream, pubsub.AppQueue, bts, header, e.cfg.GPTScript.Runner.RequestTimeout)
		if err != nil {
			log.Warn().Err(err).Str("app_repo", app.Repo).Msg("failed to request GPTScript app")
			return nil, fmt.Errorf("failed to request GPTScript app: %w", err)
		}
		return resp, nil
	},
		retry.Attempts(e.cfg.GPTScript.Runner.Retries),
		retry.Delay(delayBetweenExecuteRetries),
		retry.Context(ctx),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			retries = int(n)
			log.Warn().
				Err(err).
				Str("app_repo", app.Repo).
				Uint("retry_number", n).
				Msg("retrying app execution")
		}),
	)
	if err != nil {
		return nil, err
	}

	var response types.GptScriptResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GPTScript app response: %w", err)
	}

	response.Retries = retries

	return &response, nil
}

const (
	executeRetries             = 3
	delayBetweenExecuteRetries = 50 * time.Millisecond
)

func (e *DefaultExecutor) ExecuteScript(ctx context.Context, script *types.GptScript) (*types.GptScriptResponse, error) {
	bts, err := json.Marshal(script)
	if err != nil {
		return nil, err
	}

	var retries int

	header := map[string]string{
		"kind": "tool",
	}

	resp, err := retry.DoWithData(func() ([]byte, error) {
		resp, err := e.pubsub.StreamRequest(ctx, pubsub.ScriptRunnerStream, pubsub.AppQueue, bts, header, 30*time.Second)
		if err != nil {
			return nil, fmt.Errorf("failed to request GPTScript app: %w", err)
		}
		return resp, nil
	},
		retry.Attempts(executeRetries),
		retry.Delay(delayBetweenExecuteRetries),
		retry.Context(ctx),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			retries = int(n)
			log.Warn().
				Err(err).
				Str("script_input", script.Input).
				Uint("retry_number", n).
				Msg("retrying script execution")
		}),
	)
	if err != nil {
		return nil, err
	}

	var response types.GptScriptResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GPTScript app response: %w", err)
	}

	response.Retries = retries

	return &response, nil
}

// DirectExecutor runs GPTScript scripts directly
type DirectExecutor struct{}

var _ Executor = &TestFasterExecutor{}

func NewDirectExecutor() *DirectExecutor {
	return &DirectExecutor{}
}

func (e *DirectExecutor) ExecuteApp(ctx context.Context, app *types.GptScriptGithubApp) (*types.GptScriptResponse, error) {
	return RunGPTAppScript(ctx, app)
}

func (e *DirectExecutor) ExecuteScript(ctx context.Context, script *types.GptScript) (*types.GptScriptResponse, error) {
	return RunGPTScript(ctx, script)
}
