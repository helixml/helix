package gptscript

import (
	"context"

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

func NewExecutor(cfg *config.ServerConfig, pubsub pubsub.PubSub) *DefaultExecutor {
	return &DefaultExecutor{
		cfg:    cfg,
		pubsub: pubsub,
	}
}

func (e *DefaultExecutor) ExecuteApp(ctx context.Context, app *types.GptScriptGithubApp) (*types.GptScriptResponse, error) {

	return nil, nil
}

func (e *DefaultExecutor) ExecuteScript(ctx context.Context, script *types.GptScript) (*types.GptScriptResponse, error) {
	return nil, nil
}
