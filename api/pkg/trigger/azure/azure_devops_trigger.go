package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// AzureDevOps is triggered by webhooks from Azure DevOps
// ref: https://learn.microsoft.com/en-us/azure/devops/service-hooks/services/webhooks?view=azure-devops
type AzureDevOps struct { //nolint:revive
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller
}

func New(cfg *config.ServerConfig, store store.Store, controller *controller.Controller) *AzureDevOps {

	return &AzureDevOps{
		cfg:        cfg,
		store:      store,
		controller: controller,
	}
}

// ProcessWebhook - attempts best effort to distill the context for the LLM based on the event type. For pull requests
// we are parsing and removing most irrelevant information. For comments, similar case.
func (a *AzureDevOps) ProcessWebhook(ctx context.Context, triggerConfig *types.TriggerConfiguration, payload []byte) error {
	// First, we need to parse the payload into the Event struct
	var event Event
	err := json.Unmarshal(payload, &event)
	if err != nil {
		return err
	}

	log.Info().
		Str("trigger_config_id", triggerConfig.ID).
		Str("trigger_config_app_id", triggerConfig.AppID).
		Str("event_type", event.EventType).
		Msgf("AzureDevOps: processing webhook")

	switch event.EventType {
	case "git.pullrequest.created", "git.pullrequest.updated":
		return a.processPullRequestCreateUpdateEvent(ctx, triggerConfig, event, payload)
	case "ms.vss-code.git-pullrequest-comment-event":
		return a.processPullRequestCommentEvent(ctx, triggerConfig, event, payload)
	default:
		return a.processUnknownEvent(ctx, triggerConfig, event, payload)
	}
}

func (a *AzureDevOps) processPullRequestCreateUpdateEvent(ctx context.Context, triggerConfig *types.TriggerConfiguration, event Event, payload []byte) error {
	var pr PullRequest
	err := json.Unmarshal(payload, &pr)
	if err != nil {
		return err
	}

	rendered, err := renderPullRequestCreatedUpdatedEvent(pr)
	if err != nil {
		return fmt.Errorf("failed to render pull request created/updated event: %w", err)
	}

	// Set PR context
	ctx = types.SetAzureDevopsRepositoryContext(ctx, types.AzureDevopsRepositoryContext{
		RepositoryID:  pr.Resource.Repository.ID,
		PullRequestID: pr.Resource.PullRequestID,
		ProjectID:     pr.Resource.Repository.Project.ID,
	})

	// Process the rendered template
	return a.processEvent(ctx, triggerConfig, event, rendered)
}

func (a *AzureDevOps) processPullRequestCommentEvent(ctx context.Context, triggerConfig *types.TriggerConfiguration, event Event, payload []byte) error {
	var prc PullRequestComment
	err := json.Unmarshal(payload, &prc)
	if err != nil {
		return err
	}

	if prc.Resource.Comment.IsDeleted {
		// Nothing to do
		log.Info().
			Str("app_id", triggerConfig.AppID).
			Str("trigger_config_id", triggerConfig.ID).
			Str("event_type", event.EventType).
			Msg("AzureDevOps: pull request comment deleted, nothing to do")
		return nil
	}

	rendered, err := renderPullRequestCommentedEvent(prc)
	if err != nil {
		return fmt.Errorf("failed to unmarshal pull request comment event: %w", err)
	}

	// TODO: get messages from the thread

	// Set PR context
	ctx = types.SetAzureDevopsRepositoryContext(ctx, types.AzureDevopsRepositoryContext{
		RepositoryID:  prc.Resource.PullRequest.Repository.ID,
		PullRequestID: prc.Resource.PullRequest.PullRequestID,
		ProjectID:     prc.Resource.PullRequest.Repository.Project.ID,
		ThreadID:      getThreadID(prc),
		CommentID:     prc.Resource.Comment.ID,
	})

	// Process the rendered template
	return a.processEvent(ctx, triggerConfig, event, rendered)
}

// If we don't know how to process the event, we will it process it plain
func (a *AzureDevOps) processUnknownEvent(ctx context.Context, triggerConfig *types.TriggerConfiguration, event Event, payload []byte) error {
	return a.processEvent(ctx, triggerConfig, event, string(payload))
}

func (a *AzureDevOps) processEvent(ctx context.Context, triggerConfig *types.TriggerConfiguration, event Event, input string) error {
	app, err := a.store.GetApp(ctx, triggerConfig.AppID)
	if err != nil {
		return err
	}

	// triggerInteractionID := system.GenerateUUID()
	// assistantResponseID := system.GenerateUUID()

	// Prepare new session
	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Name:           "Azure DevOps event - " + event.EventType,
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
			// Origin: types.SessionOrigin{
			// 	Type: types.SessionOriginTypeUserCreated,
			// },
			HelixVersion: data.GetHelixVersion(),
		},
		// Interactions: []*types.Interaction{
		// 	{
		// 		ID:        triggerInteractionID,
		// 		Created:   time.Now(),
		// 		Updated:   time.Now(),
		// 		Scheduled: time.Now(),
		// 		Completed: time.Now(),
		// 		Mode:      types.SessionModeInference,
		// 		// Creator:   types.CreatorTypeUser,
		// 		State:     types.InteractionStateComplete,
		// 		Finished:  true,
		// 		Message:   input,
		// 		Content: types.MessageContent{
		// 			ContentType: types.MessageContentTypeText,
		// 			Parts:       []any{input},
		// 		},
		// 	},
		// 	{
		// 		ID:       assistantResponseID,
		// 		Created:  time.Now(),
		// 		Updated:  time.Now(),
		// 		Creator:  types.CreatorTypeAssistant,
		// 		Mode:     types.SessionModeInference,
		// 		Message:  "",
		// 		State:    types.InteractionStateWaiting,
		// 		Finished: false,
		// 		Metadata: map[string]string{},
		// 	},
		// },
	}

	// ctx = oai.SetContextSessionID(ctx, session.ID)

	// messages := []openai.ChatCompletionMessage{
	// 	{
	// 		Role:    openai.ChatMessageRoleUser,
	// 		Content: input,
	// 	},
	// }

	// request := openai.ChatCompletionRequest{
	// 	Stream:   false,
	// 	Messages: messages,
	// }

	// bts, err := json.MarshalIndent(request, "", "  ")
	// if err != nil {
	// 	log.Error().
	// 		Err(err).
	// 		Str("app_id", app.ID).
	// 		Msg("failed to marshal request")
	// }

	// ctx = oai.SetContextValues(ctx, &oai.ContextValues{
	// 	OwnerID:         app.Owner,
	// 	SessionID:       session.ID,
	// 	InteractionID:   assistantResponseID,
	// 	OriginalRequest: bts,
	// })

	// ctx = oai.SetContextAppID(ctx, app.ID)
	// ctx = oai.SetContextOrganizationID(ctx, app.OrganizationID)

	// Write session to the database
	err = a.controller.WriteSession(ctx, session)
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Msg("failed to create session")
		return fmt.Errorf("failed to create session: %w", err)
	}

	user, err := a.store.GetUser(ctx, &store.GetUserQuery{
		ID: app.Owner,
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Str("user_id", app.Owner).
			Msg("failed to get user")
		return fmt.Errorf("failed to get user: %w", err)
	}

	resp, err := a.controller.RunBlockingSession(ctx, &controller.RunSessionRequest{
		OrganizationID: app.OrganizationID,
		App:            app,
		Session:        session,
		User:           user,
		PromptMessage:  types.MessageContent{Parts: []any{input}},
	})
	if err != nil {
		log.Warn().
			Err(err).
			Str("app_id", app.ID).
			Msg("failed to run app cron job")

		// Update session with error
		// session.Interactions[len(session.Interactions)-1].Error = err.Error()
		// session.Interactions[len(session.Interactions)-1].State = types.InteractionStateError
		// session.Interactions[len(session.Interactions)-1].Finished = true
		// session.Interactions[len(session.Interactions)-1].Completed = time.Now()
		// err = a.controller.WriteSession(ctx, session)
		// if err != nil {
		// 	log.Error().
		// 		Err(err).
		// 		Str("app_id", app.ID).
		// 		Str("user_id", app.Owner).
		// 		Str("session_id", session.ID).
		// 		Msg("failed to update session")
		// }
		return fmt.Errorf("failed to run app cron job: %w", err)
	}

	log.Info().
		Str("app_id", app.ID).
		Str("resp_content", resp.ResponseMessage).
		Msg("Azure DevOps event processed")

	return nil
}
