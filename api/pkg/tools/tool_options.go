package tools

// Option is a function on the options for a connection.
type Option func(*Options) error

// Options can be used to create a customized connection.
type Options struct {
	isActionableTemplate string
	model                string
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
