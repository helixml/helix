package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

const (
	defaultExternalAgentModel        = "gpt-4"
	defaultExternalAgentReadyTimeout = 300 * time.Second
	defaultExternalAgentWaitTimeout  = 180 * time.Second
)

type ExternalAgentMode string

const (
	ExternalAgentModeBlocking  ExternalAgentMode = "blocking"
	ExternalAgentModeStreaming ExternalAgentMode = "streaming"
)

type ExternalAgentHooks struct {
	WaitForExternalAgentReady func(ctx context.Context, sessionID string, timeout time.Duration) error
	GetAgentNameForSession    func(ctx context.Context, session *types.Session) string
	SendCommand               func(sessionID string, command types.ExternalAgentCommand) error
	StoreResponseChannel      func(sessionID, requestID string, responseChan chan string, doneChan chan bool, errorChan chan error)
	CleanupResponseChannel    func(sessionID, requestID string)
	SetWaitingInteraction     func(sessionID, interactionID string)
	SetRequestSessionMapping  func(requestID, sessionID string)
}

type RunExternalAgentRequest struct {
	Session               *types.Session
	ChatCompletionRequest openai.ChatCompletionRequest
	Mode                  ExternalAgentMode
	Start                 time.Time
	ReadyTimeout          time.Duration
	ResponseTimeout       time.Duration
}

type ExternalAgentStream struct {
	Chunks <-chan string
	Done   <-chan struct{}
	Errors <-chan error
}

type RunExternalAgentResponse struct {
	RequestID    string
	Model        string
	FullResponse string
	Stream       *ExternalAgentStream
}

func (c *Controller) RunExternalAgent(ctx context.Context, req RunExternalAgentRequest) (*RunExternalAgentResponse, error) {
	if req.Session == nil {
		return nil, fmt.Errorf("session is required")
	}
	if c.Options.ExternalAgentExecutor == nil {
		return nil, fmt.Errorf("external agent executor not available")
	}
	if len(req.Session.Interactions) == 0 {
		return nil, fmt.Errorf("no interactions found in session")
	}

	if req.Mode == "" {
		req.Mode = ExternalAgentModeBlocking
	}
	if req.Start.IsZero() {
		req.Start = time.Now()
	}
	if req.ReadyTimeout <= 0 {
		req.ReadyTimeout = defaultExternalAgentReadyTimeout
	}
	if req.ResponseTimeout <= 0 {
		req.ResponseTimeout = defaultExternalAgentWaitTimeout
	}

	hooks := c.GetExternalAgentHooks()

	if hooks.WaitForExternalAgentReady == nil ||
		hooks.SendCommand == nil ||
		hooks.StoreResponseChannel == nil ||
		hooks.CleanupResponseChannel == nil ||
		hooks.SetWaitingInteraction == nil ||
		hooks.SetRequestSessionMapping == nil {
		return nil, fmt.Errorf("external agent hooks are incomplete")
	}

	agentSession, err := c.Options.ExternalAgentExecutor.GetSession(req.Session.ID)
	if err != nil {
		return nil, fmt.Errorf("external agent session not found: %w", err)
	}

	if req.ChatCompletionRequest.Model == "" {
		req.ChatCompletionRequest.Model = defaultExternalAgentModel
	}

	userMessage := extractExternalAgentUserMessage(req.ChatCompletionRequest)
	if userMessage == "" {
		return nil, fmt.Errorf("no user message found")
	}

	interaction := req.Session.Interactions[len(req.Session.Interactions)-1]

	log.Info().
		Str("session_id", req.Session.ID).
		Str("user_message", userMessage).
		Str("agent_session_status", agentSession.Status).
		Str("mode", string(req.Mode)).
		Msg("sending message to external agent")

	if err := hooks.WaitForExternalAgentReady(ctx, req.Session.ID, req.ReadyTimeout); err != nil {
		c.markExternalAgentInteractionError(req.Session, interaction, req.Start, fmt.Sprintf("External agent not ready: %s", err.Error()))
		return nil, fmt.Errorf("external agent not ready: %w", err)
	}

	requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
	agentName := "zed-agent"
	if hooks.GetAgentNameForSession != nil {
		agentName = hooks.GetAgentNameForSession(ctx, req.Session)
	}

	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"acp_thread_id": req.Session.Metadata.ZedThreadID,
			"message":       userMessage,
			"request_id":    requestID,
			"agent_name":    agentName,
		},
	}

	responseChan := make(chan string, 100)
	doneChan := make(chan bool, 1)
	errorChan := make(chan error, 1)

	hooks.StoreResponseChannel(req.Session.ID, requestID, responseChan, doneChan, errorChan)
	hooks.SetWaitingInteraction(req.Session.ID, interaction.ID)
	hooks.SetRequestSessionMapping(requestID, req.Session.ID)

	if err := hooks.SendCommand(req.Session.ID, command); err != nil {
		hooks.CleanupResponseChannel(req.Session.ID, requestID)
		c.markExternalAgentInteractionError(req.Session, interaction, req.Start, err.Error())
		return nil, fmt.Errorf("failed to send command to external agent: %w", err)
	}

	if req.Mode == ExternalAgentModeStreaming {
		stream := c.startExternalAgentStreamWorker(ctx, req, hooks, requestID, interaction, responseChan, doneChan, errorChan)
		return &RunExternalAgentResponse{
			RequestID: requestID,
			Model:     req.ChatCompletionRequest.Model,
			Stream:    stream,
		}, nil
	}

	fullResponse, err := c.waitForExternalAgentResponse(ctx, req, requestID, interaction, responseChan, doneChan, errorChan, nil)
	hooks.CleanupResponseChannel(req.Session.ID, requestID)
	if err != nil {
		return nil, err
	}

	return &RunExternalAgentResponse{
		RequestID:    requestID,
		Model:        req.ChatCompletionRequest.Model,
		FullResponse: fullResponse,
	}, nil
}

func (c *Controller) startExternalAgentStreamWorker(
	ctx context.Context,
	req RunExternalAgentRequest,
	hooks ExternalAgentHooks,
	requestID string,
	interaction *types.Interaction,
	responseChan chan string,
	doneChan chan bool,
	errorChan chan error,
) *ExternalAgentStream {
	chunksOut := make(chan string, 32)
	doneOut := make(chan struct{}, 1)
	errorsOut := make(chan error, 1)

	go func() {
		defer close(chunksOut)
		defer close(doneOut)
		defer close(errorsOut)
		defer hooks.CleanupResponseChannel(req.Session.ID, requestID)

		_, err := c.waitForExternalAgentResponse(ctx, req, requestID, interaction, responseChan, doneChan, errorChan, func(chunk string) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case chunksOut <- chunk:
				return nil
			}
		})
		if err != nil {
			errorsOut <- err
			return
		}

		doneOut <- struct{}{}
	}()

	return &ExternalAgentStream{
		Chunks: chunksOut,
		Done:   doneOut,
		Errors: errorsOut,
	}
}

func (c *Controller) SetExternalAgentHooks(hooks ExternalAgentHooks) {
	c.externalAgentHooksMu.Lock()
	c.externalAgentHooks = hooks
	c.externalAgentHooksMu.Unlock()
}

func (c *Controller) GetExternalAgentHooks() ExternalAgentHooks {
	c.externalAgentHooksMu.RLock()
	hooks := c.externalAgentHooks
	c.externalAgentHooksMu.RUnlock()
	return hooks
}

func (c *Controller) waitForExternalAgentResponse(
	ctx context.Context,
	req RunExternalAgentRequest,
	requestID string,
	interaction *types.Interaction,
	responseChan chan string,
	doneChan chan bool,
	errorChan chan error,
	onChunk func(chunk string) error,
) (string, error) {
	timeout := time.NewTimer(req.ResponseTimeout)
	defer timeout.Stop()

	var fullResponse string
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case chunk := <-responseChan:
			fullResponse += chunk
			if onChunk != nil {
				if err := onChunk(chunk); err != nil {
					return "", err
				}
			}
		case <-doneChan:
			reloadCtx, cancelReload := context.WithTimeout(context.Background(), 5*time.Second)
			reloadedInteraction, err := c.Options.Store.GetInteraction(reloadCtx, interaction.ID)
			cancelReload()
			if err == nil {
				interaction.ResponseMessage = reloadedInteraction.ResponseMessage
			} else if interaction.ResponseMessage == "" {
				interaction.ResponseMessage = fullResponse
			}

			interaction.Completed = time.Now()
			interaction.State = types.InteractionStateComplete
			interaction.DurationMs = int(time.Since(req.Start).Milliseconds())

			updateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			updateErr := c.UpdateInteraction(updateCtx, req.Session, interaction)
			cancel()
			if updateErr != nil {
				log.Error().Err(updateErr).Str("session_id", req.Session.ID).Msg("failed to update interaction")
			}

			return interaction.ResponseMessage, nil
		case err := <-errorChan:
			c.markExternalAgentInteractionError(req.Session, interaction, req.Start, err.Error())
			return "", fmt.Errorf("external agent error: %w", err)
		case <-timeout.C:
			c.markExternalAgentInteractionError(req.Session, interaction, req.Start, "External agent response timeout")
			return "", fmt.Errorf("external agent response timeout")
		}
	}
}

func (c *Controller) markExternalAgentInteractionError(session *types.Session, interaction *types.Interaction, start time.Time, errorMessage string) {
	interaction.Error = errorMessage
	interaction.State = types.InteractionStateError
	interaction.Completed = time.Now()
	interaction.DurationMs = int(time.Since(start).Milliseconds())

	updateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.UpdateInteraction(updateCtx, session, interaction); err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("failed to update interaction")
	}
}

func extractExternalAgentUserMessage(chatCompletionRequest openai.ChatCompletionRequest) string {
	if len(chatCompletionRequest.Messages) == 0 {
		return ""
	}

	lastMessage := chatCompletionRequest.Messages[len(chatCompletionRequest.Messages)-1]
	if lastMessage.Role != openai.ChatMessageRoleUser {
		return ""
	}

	if lastMessage.Content != "" {
		return lastMessage.Content
	}

	var userMessage string
	for _, part := range lastMessage.MultiContent {
		if part.Type == openai.ChatMessagePartTypeText {
			userMessage += part.Text
		}
	}
	return userMessage
}
