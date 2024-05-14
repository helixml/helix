package gptscript

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
)

//go:generate mockgen -source $GOFILE -destination gptscript_mocks.go -package $GOPACKAGE

type Executor interface {
	ExecuteApp(ctx context.Context, app *types.GptScriptGithubApp) (*types.GptScriptResponse, error)
	ExecuteScript(ctx context.Context, script *types.GptScript) (*types.GptScriptResponse, error)
}

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

	resp, err := e.pubsub.Request(ctx, pubsub.GetGPTScriptAppQueue(), bts, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to request GPTScript app: %w", err)
	}

	var response types.GptScriptResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GPTScript app response: %w", err)
	}

	return &response, nil
}

func (e *DefaultExecutor) ExecuteScript(ctx context.Context, script *types.GptScript) (*types.GptScriptResponse, error) {
	bts, err := json.Marshal(script)
	if err != nil {
		return nil, err
	}

	resp, err := e.pubsub.Request(ctx, pubsub.GetGPTScriptToolQueue(), bts, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to request GPTScript app: %w", err)
	}

	var response types.GptScriptResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GPTScript app response: %w", err)
	}

	return &response, nil
}

type TestFasterExecutor struct{}

var _ Executor = &TestFasterExecutor{}

func NewTestFasterExecutor() *TestFasterExecutor {
	return &TestFasterExecutor{}
}

func (e *TestFasterExecutor) ExecuteApp(ctx context.Context, app *types.GptScriptGithubApp) (*types.GptScriptResponse, error) {
	return RunGPTAppTestfaster(ctx, app)
}

func (e *TestFasterExecutor) ExecuteScript(ctx context.Context, script *types.GptScript) (*types.GptScriptResponse, error) {
	return RunGPTScriptTestfaster(ctx, script)
}
