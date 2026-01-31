package tools

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"sync"

	"github.com/rs/zerolog/log"
	oai "github.com/sashabaranov/go-openai"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// TODO: probably move planner into a separate package so we can decide when we want to call APIs, when to go with RAG, etc.
type Planner interface {
	IsActionable(ctx context.Context, sessionID, interactionID string, tools []*types.Tool, history []*types.ToolHistoryMessage, options ...Option) (*IsActionableResponse, error)
	// TODO: RAG lookup
	RunAction(ctx context.Context, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, action string, options ...Option) (*RunActionResponse, error)
	RunActionStream(ctx context.Context, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, action string, options ...Option) (*oai.ChatCompletionStream, error)
	// Validation and defaulting
	ValidateAndDefault(ctx context.Context, tool *types.Tool) (*types.Tool, error)

	// Low level methods for Model Context Protocol (MCP)
	RunAPIActionWithParameters(ctx context.Context, req *types.RunAPIActionRequest, options ...Option) (*types.RunAPIActionResponse, error)
}

// Static check
var _ Planner = &ChainStrategy{}

type ChainStrategy struct {
	oauthManager *oauth.Manager `json:"-"`
	sessionStore store.Store    `json:"-"`
	appStore     store.Store    `json:"-"`

	cfg   *config.ServerConfig
	store store.Store

	apiClient                 openai.Client // Default API client is none is passed through the options
	httpClient                *http.Client
	isActionableTemplate      string
	isActionableHistoryLength int
	wg                        sync.WaitGroup
}

func NewChainStrategy(cfg *config.ServerConfig, store store.Store, client openai.Client) (*ChainStrategy, error) {
	isActionableTemplate, err := getIsActionablePromptTemplate(cfg)
	if err != nil {
		log.Err(err).Msg("failed to get actionable template, falling back to default")
		// Use default so things don't break
		isActionableTemplate = isInformativeOrActionablePrompt
	}

	retryClient := system.NewRetryClient(3, cfg.Tools.TLSSkipVerify)

	return &ChainStrategy{
		cfg:                       cfg,
		store:                     store,
		apiClient:                 client,
		httpClient:                retryClient.StandardClient(),
		isActionableTemplate:      isActionableTemplate,
		isActionableHistoryLength: cfg.Tools.IsActionableHistoryLength,
	}, nil
}

func getIsActionablePromptTemplate(cfg *config.ServerConfig) (string, error) {
	if cfg.Tools.IsActionableTemplate == "" {
		return isInformativeOrActionablePrompt, nil
	}

	// If path - read it
	if _, err := os.Stat(cfg.Tools.IsActionableTemplate); err == nil {
		bts, err := os.ReadFile(cfg.Tools.IsActionableTemplate)
		if err != nil {
			return "", err
		}

		return string(bts), nil
	}

	decoded, err := base64.StdEncoding.DecodeString(cfg.Tools.IsActionableTemplate)
	if err != nil {
		return cfg.Tools.IsActionableTemplate, nil
	}

	return string(decoded), nil
}
