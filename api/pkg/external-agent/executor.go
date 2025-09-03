package external_agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
)

//go:generate mockgen -source $GOFILE -destination executor_mocks.go -package $GOPACKAGE

// Executor interface for executing external agents
type Executor interface {
	StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error)
	StopZedAgent(ctx context.Context, sessionID string) error
	GetSession(sessionID string) (*ZedSession, bool)
	ListSessions() []*ZedSession
}

// DefaultExecutor runs external agents through the Helix control-plane
type DefaultExecutor struct {
	cfg    *config.ServerConfig
	pubsub pubsub.PubSub
}

var _ Executor = &DefaultExecutor{}

func NewExecutor(cfg *config.ServerConfig, pubsub pubsub.PubSub) *DefaultExecutor {
	return &DefaultExecutor{
		cfg:    cfg,
		pubsub: pubsub,
	}
}

const (
	executeRetries             = 3
	delayBetweenExecuteRetries = 50 * time.Millisecond
)

func (e *DefaultExecutor) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	bts, err := json.Marshal(agent)
	if err != nil {
		return nil, err
	}

	var retries int

	resp, err := retry.DoWithData(func() ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		header := map[string]string{
			"kind": "zed_agent",
		}

		resp, err := e.pubsub.StreamRequest(ctx, pubsub.ExternalAgentRunnerStream, pubsub.ExternalAgentQueue, bts, header, 30*time.Second)
		if err != nil {
			log.Warn().Err(err).Str("session_id", agent.SessionID).Msg("failed to request Zed agent")
			return nil, fmt.Errorf("failed to request Zed agent: %w", err)
		}
		return resp, nil
	},
		retry.Attempts(executeRetries),
		retry.Delay(delayBetweenExecuteRetries),
		retry.Context(ctx),
		retry.LastErrorOnly(true),
		retry.OnRetry(func(n uint, err error) {
			retries = int(n)
			log.Warn().
				Err(err).
				Str("session_id", agent.SessionID).
				Uint("retry_number", n).
				Msg("retrying Zed agent execution")
		}),
	)
	if err != nil {
		return nil, err
	}

	var response types.ZedAgentResponse
	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Zed agent response: %w", err)
	}

	response.Retries = retries

	return &response, nil
}

func (e *DefaultExecutor) StopZedAgent(ctx context.Context, sessionID string) error {
	// For now, send a stop request through pub/sub
	// In a full implementation, we could send stop commands to specific runners
	log.Info().Str("session_id", sessionID).Msg("stopping Zed agent via pub/sub")

	stopRequest := map[string]string{
		"action":     "stop",
		"session_id": sessionID,
	}

	bts, err := json.Marshal(stopRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal stop request: %w", err)
	}

	header := map[string]string{
		"kind": "stop_agent",
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err = e.pubsub.StreamRequest(ctx, pubsub.ExternalAgentRunnerStream, pubsub.ExternalAgentQueue, bts, header, 10*time.Second)
	return err
}

func (e *DefaultExecutor) GetSession(sessionID string) (*ZedSession, bool) {
	// TODO: Implement session retrieval through pub/sub
	// For now, return not found since we don't have session state in the executor
	return nil, false
}

func (e *DefaultExecutor) ListSessions() []*ZedSession {
	// TODO: Implement session listing through pub/sub
	// For now, return empty list since we don't have session state in the executor
	return nil
}

// DirectExecutor runs external agents directly in the same process
type DirectExecutor struct {
	zedExecutor *ZedExecutor
}

func NewDirectExecutor(displayBase, portBase int, workspaceBase, rdpUser, rdpPassword string) *DirectExecutor {
	return &DirectExecutor{
		zedExecutor: NewZedExecutor(displayBase, portBase, workspaceBase, rdpUser, rdpPassword),
	}
}

func (e *DirectExecutor) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	return e.zedExecutor.StartZedAgent(ctx, agent)
}

func (e *DirectExecutor) StopZedAgent(ctx context.Context, sessionID string) error {
	return e.zedExecutor.StopZedAgent(ctx, sessionID)
}

func (e *DirectExecutor) GetSession(sessionID string) (*ZedSession, bool) {
	return e.zedExecutor.GetSession(sessionID)
}

func (e *DirectExecutor) ListSessions() []*ZedSession {
	return e.zedExecutor.ListSessions()
}
