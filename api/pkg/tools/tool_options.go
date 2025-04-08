package tools

import (
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/openai"
)

// Option is a function on the options for a connection.
type Option func(*Options) error

// Options can be used to create a customized connection.
type Options struct {
	isActionableTemplate      string
	isActionableHistoryLength int
	model                     string
	client                    openai.Client
	oauthTokens               map[string]string
	oauthManager              *oauth.Manager
	//owner               string // For later
}

func WithIsActionableTemplate(isActionableTemplate string) Option {
	return func(o *Options) error {
		o.isActionableTemplate = isActionableTemplate
		return nil
	}
}

func WithIsActionableHistoryLength(isActionableHistoryLength int) Option {
	return func(o *Options) error {
		o.isActionableHistoryLength = isActionableHistoryLength
		return nil
	}
}

func WithModel(model string) Option {
	return func(o *Options) error {
		o.model = model
		return nil
	}
}

func WithClient(client openai.Client) Option {
	return func(o *Options) error {
		o.client = client
		return nil
	}
}

func WithOAuthTokens(tokens map[string]string) Option {
	return func(o *Options) error {
		o.oauthTokens = tokens
		return nil
	}
}

func WithOAuthManager(manager *oauth.Manager) Option {
	return func(o *Options) error {
		o.oauthManager = manager
		return nil
	}
}
