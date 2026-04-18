package evaluation

//go:generate mockgen -source $GOFILE -destination mocks_generated.go -package $GOPACKAGE

import (
	"context"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
)

// SessionController abstracts the controller methods needed by the evaluation runner
type SessionController interface {
	WriteSession(ctx context.Context, session *types.Session) error
	RunBlockingSession(ctx context.Context, req *controller.RunSessionRequest) (*types.Interaction, error)
}

// ChatCompleter abstracts the controller's ChatCompletion for the LLM judge
type ChatCompleter interface {
	ChatCompletion(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *controller.ChatCompletionOptions) (*openai.ChatCompletionResponse, *openai.ChatCompletionRequest, error)
}
