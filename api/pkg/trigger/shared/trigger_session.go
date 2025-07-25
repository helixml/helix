package shared

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/data"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/sashabaranov/go-openai"
)

type TriggerSession struct {
	Session               *types.Session
	TriggerInteractionID  string          // Trigger interaction ID, can be cron, discord, slack, etc.
	AssistantResponseID   string          // Agent response ID
	RequestContext        context.Context // Context with the metadata for the request
	ChatCompletionRequest openai.ChatCompletionRequest
}

func NewTriggerSession(ctx context.Context, triggerName string, app *types.App, input string) *TriggerSession {
	triggerInteractionID := system.GenerateUUID()
	assistantResponseID := system.GenerateUUID()

	// Prepare new session
	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Name:           triggerName,
		Created:        time.Now(),
		Updated:        time.Now(),
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		ParentApp:      app.ID,
		OrganizationID: app.OrganizationID,
		Owner:          app.Owner,
		OwnerType:      app.OwnerType,
		Metadata: types.SessionMetadata{
			Stream:       false,
			SystemPrompt: "",
			AssistantID:  "",
			Origin: types.SessionOrigin{
				Type: types.SessionOriginTypeUserCreated,
			},
			HelixVersion: data.GetHelixVersion(),
		},
		Interactions: []*types.Interaction{
			{
				ID:        triggerInteractionID,
				Created:   time.Now(),
				Updated:   time.Now(),
				Scheduled: time.Now(),
				Completed: time.Now(),
				Mode:      types.SessionModeInference,
				Creator:   types.CreatorTypeUser,
				State:     types.InteractionStateComplete,
				Finished:  true,
				Message:   input,
				Content: types.MessageContent{
					ContentType: types.MessageContentTypeText,
					Parts:       []any{input},
				},
			},
			{
				ID:       assistantResponseID,
				Created:  time.Now(),
				Updated:  time.Now(),
				Creator:  types.CreatorTypeAssistant,
				Mode:     types.SessionModeInference,
				Message:  "",
				State:    types.InteractionStateWaiting,
				Finished: false,
				Metadata: map[string]string{},
			},
		},
	}

	ctx = oai.SetContextSessionID(ctx, session.ID)

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: input,
		},
	}

	request := openai.ChatCompletionRequest{
		Stream:   false,
		Messages: messages,
	}

	bts, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Msg("failed to marshal request")
	}

	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:         app.Owner,
		SessionID:       session.ID,
		InteractionID:   assistantResponseID,
		OriginalRequest: bts,
	})

	ctx = oai.SetContextAppID(ctx, app.ID)

	return &TriggerSession{
		Session:               session,
		TriggerInteractionID:  triggerInteractionID,
		AssistantResponseID:   assistantResponseID,
		RequestContext:        ctx,
		ChatCompletionRequest: request,
	}
}
