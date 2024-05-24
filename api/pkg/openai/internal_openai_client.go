package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"

	openai "github.com/lukemarsden/go-openai2"
)

// InternalHelixClient utilizes Helix runners to complete chat requests. Primary
// purpose is to power internal tools
type InternalHelixClient struct {
	cfg *config.ServerConfig

	pubsub     pubsub.PubSub // Used to get responses from the runners
	controller Controller    // Used to create sessions
}

func NewInternalHelixClient(cfg *config.ServerConfig, pubsub pubsub.PubSub, controller Controller) *InternalHelixClient {
	return &InternalHelixClient{
		cfg:        cfg,
		pubsub:     pubsub,
		controller: controller,
	}
}

func (c *InternalHelixClient) CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (resp openai.ChatCompletionResponse, err error) {
	sessionID := system.GenerateUUID()

	doneCh := make(chan struct{})

	var updatedSession *types.Session

	sub, err := c.pubsub.Subscribe(ctx, pubsub.GetSessionQueue(c.cfg.Providers.Helix.OwnerID, sessionID), func(payload []byte) error {
		var event types.WebsocketEvent
		err := json.Unmarshal(payload, &event)
		if err != nil {
			return fmt.Errorf("error unmarshalling websocket event '%s': %w", string(payload), err)
		}

		if event.Type != "session_update" || event.Session == nil {
			return nil
		}

		if event.Session.Interactions[len(event.Session.Interactions)-1].State == types.InteractionStateComplete {
			// We are done
			updatedSession = event.Session

			close(doneCh)
			return nil
		}

		// Continue reading
		return nil
	})
	if err != nil {
		log.Err(err).Msg("failed to subscribe to session updates")
		return openai.ChatCompletionResponse{}, fmt.Errorf("failed to subscribe to session updates: %w", err)
	}

	// Start the session
	err = c.startSession(ctx, &request, sessionID)
	if err != nil {
		log.Err(err).Msg("failed to start session")
		return openai.ChatCompletionResponse{}, fmt.Errorf("failed to start session: %w", err)
	}

	// Wait for response

	select {
	case <-doneCh:
		_ = sub.Unsubscribe()
		// Continue with response
	case <-ctx.Done():
		_ = sub.Unsubscribe()
		return
	}

	if updatedSession == nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("session update not received")
	}

	if updatedSession.Interactions == nil || len(updatedSession.Interactions) == 0 {
		return openai.ChatCompletionResponse{}, fmt.Errorf("session update does not contain any interactions")
	}

	var result []openai.ChatCompletionChoice

	// Take the last interaction
	interaction := updatedSession.Interactions[len(updatedSession.Interactions)-1]

	result = append(result, openai.ChatCompletionChoice{
		Message: openai.ChatCompletionMessage{
			Role:    "assistant",
			Content: interaction.Message,
		},
		FinishReason: "stop",
	})

	resp = openai.ChatCompletionResponse{
		ID:      sessionID,
		Created: int64(time.Now().Unix()),
		Model:   string(request.Model),
		Choices: result,
		Object:  "chat.completion",
		Usage: openai.Usage{
			// TODO: calculate
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
	}

	return resp, nil
}

func (c *InternalHelixClient) startSession(ctx context.Context, req *openai.ChatCompletionRequest, sessionID string) error {

	sessionMode := types.SessionModeInference

	var interactions []*types.Interaction

	for _, m := range req.Messages {
		// Validating roles
		switch m.Role {
		case "user", "system", "assistant":
			// OK
		default:
			return fmt.Errorf("invalid role '%s', available roles: 'user', 'system', 'assistant'", m.Role)
		}

		var creator types.CreatorType
		switch m.Role {
		case "user":
			creator = types.CreatorTypeUser
		case "system":
			creator = types.CreatorTypeSystem
		}

		interaction := &types.Interaction{
			ID:             system.GenerateUUID(),
			Created:        time.Now(),
			Updated:        time.Now(),
			Scheduled:      time.Now(),
			Completed:      time.Now(),
			Creator:        creator,
			Mode:           sessionMode,
			Message:        m.Content,
			Files:          []string{},
			State:          types.InteractionStateComplete,
			Finished:       true,
			Metadata:       map[string]string{},
			DataPrepChunks: map[string][]types.DataPrepChunk{},
		}

		interactions = append(interactions, interaction)
	}

	createSessionReq := types.InternalSessionRequest{
		ID:               sessionID,
		Mode:             sessionMode,
		Type:             types.SessionTypeText,
		Stream:           req.Stream,
		ModelName:        types.ModelName(req.Model),
		Owner:            c.cfg.Providers.Helix.OwnerID,
		OwnerType:        types.OwnerType(c.cfg.Providers.Helix.OwnerType),
		UserInteractions: interactions,
		Priority:         true, // TODO: maybe get from config
		ActiveTools:      []string{},
	}

	if req.ResponseFormat != nil {
		createSessionReq.ResponseFormat = types.ResponseFormat{
			Type:   types.ResponseFormatType(req.ResponseFormat.Type),
			Schema: req.ResponseFormat.Schema,
		}
	}

	_, err := c.controller.StartSession(types.RequestContext{
		Ctx: ctx,
		User: types.User{
			ID:    createSessionReq.Owner,
			Type:  createSessionReq.OwnerType,
			Admin: false,
			Email: "system@helix.ml",
		},
	}, createSessionReq)
	return err
}

type Controller interface {
	StartSession(ctx types.RequestContext, req types.InternalSessionRequest) (*types.Session, error)
}
