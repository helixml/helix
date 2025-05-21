package agent

import (
	"context"
	"encoding/json"
	"fmt"

	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

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
}

var _ StepInfoEmitter = &PubSubStepInfoEmitter{}

func NewPubSubStepInfoEmitter(pubsub pubsub.PubSub) *PubSubStepInfoEmitter {
	return &PubSubStepInfoEmitter{
		pubsub: pubsub,
	}
}

func (p *PubSubStepInfoEmitter) EmitStepInfo(ctx context.Context, info *types.StepInfo) error {
	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		log.Warn().Msg("context values with session info not found")
		return fmt.Errorf("context values with session info not found")
	}

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

	log.Trace().
		Str("queue", queue).
		Str("step_name", info.Name).
		Str("step_message", info.Message).
		Msg("emitting step info")

	return p.pubsub.Publish(ctx, queue, bts)
}
