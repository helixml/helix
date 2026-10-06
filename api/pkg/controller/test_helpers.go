package controller

// NewInferenceTestController builds a Controller wired for the /v1/chat/completions
// path only. NewController requires a filestore, extractor and janitor that the
// inference path never touches, so tests that drive real inference through a fake
// provider construct the controller here instead.
//
// It exists because providerManager is private and is otherwise only populated by
// NewController: a bare &Controller{Options: ...} literal leaves it nil and
// getClient panics.
func NewInferenceTestController(options Options) *Controller {
	return &Controller{
		Options:         options,
		providerManager: options.ProviderManager,
	}
}
