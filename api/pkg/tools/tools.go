package tools

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"sync"

	"github.com/rs/zerolog/log"
	oai "github.com/sashabaranov/go-openai"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/netguard"
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
	retryClient.HTTPClient.Transport = apiToolTransport(cfg)
	checkRetry := retryClient.CheckRetry
	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if errors.Is(err, netguard.ErrBlockedDestination) {
			return false, err // a policy rejection will not change on retry
		}
		return checkRetry(ctx, resp, err)
	}

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

// apiToolTransport returns the transport for requests to API tools. Their URLs
// are user-defined, so every connection (redirects and DNS rebinding
// included) goes through the outbound policy.
func apiToolTransport(cfg *config.ServerConfig) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.Tools.TLSSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	netguard.Policy{AllowPrivate: cfg.Tools.AllowPrivateNetworks}.GuardTransport(transport)
	return transport
}
