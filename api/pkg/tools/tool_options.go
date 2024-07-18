package tools

type isActionableOptions struct {
	isActionableTemplate string
}

// Option is a function on the options for a connection.
type Option func(*Options) error

// Options can be used to create a customized connection.
type Options struct {
	isActionableTemplate string
}

func WithIsActionableTemplate(isActionableTemplate string) Option {
	return func(o *Options) error {
		o.isActionableTemplate = isActionableTemplate
		return nil
	}
}
