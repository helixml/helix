package tools

import "github.com/helixml/helix/api/pkg/openai"

// Option is a function on the options for a connection.
type Option func(*Options) error

// Options can be used to create a customized connection.
type Options struct {
	isActionableTemplate string
	model                string
	owner                string
	client               openai.Client
}

func WithIsActionableTemplate(isActionableTemplate string) Option {
	return func(o *Options) error {
		o.isActionableTemplate = isActionableTemplate
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
