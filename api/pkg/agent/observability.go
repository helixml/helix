package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

//go:generate mockgen -source $GOFILE -destination observability_mocks.go -package $GOPACKAGE

type StepInfoEmitter interface {
	EmitStepInfo(ctx context.Context, info *types.StepInfo) error
}

type LogStepInfoEmitter struct {
}

var _ StepInfoEmitter = &LogStepInfoEmitter{}

func NewLogStepInfoEmitter() *LogStepInfoEmitter {
	return &LogStepInfoEmitter{}
}

func (l *LogStepInfoEmitter) EmitStepInfo(ctx context.Context, info *types.StepInfo) error {
	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		log.Warn().Msg("context values with session info not found")
		return fmt.Errorf("context values with session info not found")
	}
	log.Info().
		Str("session_id", vals.SessionID).
		Str("interaction_id", vals.InteractionID).
		Str("step_name", info.Name).
		Str("step_message", info.Message).
		Msg("step info")
	return nil
}

type PubSubStepInfoEmitter struct {
	pubsub pubsub.PubSub
	store  store.Store
}

var _ StepInfoEmitter = &PubSubStepInfoEmitter{}

func NewPubSubStepInfoEmitter(pubsub pubsub.PubSub, store store.Store) *PubSubStepInfoEmitter {
	return &PubSubStepInfoEmitter{
		pubsub: pubsub,
		store:  store,
	}
}

func (p *PubSubStepInfoEmitter) EmitStepInfo(ctx context.Context, info *types.StepInfo) error {
	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		log.Warn().Msg("context values with session info not found")
		return fmt.Errorf("context values with session info not found")
	}

	appID, ok := oai.GetContextAppID(ctx)
	if !ok {
		log.Warn().Msg("appID not set in context, using 'n/a'")
		appID = "n/a"
	}

	// Only set created time if it's not already set
	if info.Created.IsZero() {
		info.Created = time.Now()
	}

	// Adding context metadata to the step info
	info.ID = system.GenerateStepInfoID()
	info.SessionID = vals.SessionID
	info.InteractionID = vals.InteractionID
	info.AppID = appID

	queue := pubsub.GetSessionQueue(vals.OwnerID, vals.SessionID)
	event := &types.WebsocketEvent{
		Type:          types.WebsocketEventProcessingStepInfo,
		SessionID:     vals.SessionID,
		InteractionID: vals.InteractionID,
		Owner:         vals.OwnerID,
		StepInfo:      info,
	}
	bts, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal step info: %w", err)
	}

	// Create new context with a timeout for persisting step info to the database.
	// Do not inherit the context from the caller, as it may be cancelled.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Saving step info to the database
	_, err = p.store.CreateStepInfo(ctx, info)
	if err != nil {
		log.Error().
			Str("session_id", vals.SessionID).
			Str("interaction_id", vals.InteractionID).
			Str("step_id", info.ID).
			Str("step_name", info.Name).
			Str("step_type", string(info.Type)).
			Msgf("failed to create step info: %v", err)
	}

	log.Info().
		Str("session_id", vals.SessionID).
		Str("interaction_id", vals.InteractionID).
		Str("queue", queue).
		Str("step_id", info.ID).
		Str("step_name", info.Name).
		Str("step_type", string(info.Type)).
		Msg("emitting step info")

	return p.pubsub.Publish(ctx, queue, bts)
}
