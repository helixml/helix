// Package session provides the Session struct for per-conversation state,
// along with methods for handling user messages and producing agent outputs.
package agent

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"
)

type Meta struct {
	AppID         string
	UserID        string
	UserEmail     string
	SessionID     string
	InteractionID string
	Extra         map[string]string
}

// Session holds ephemeral conversation data & references to global resources.
type Session struct {
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once

	inUserChannel  chan string
	outUserChannel chan Response

	llm             *LLM
	memory          Memory
	knowledgeBlock  *MemoryBlock
	agent           *Agent
	messageHistory  *MessageList
	stepInfoEmitter StepInfoEmitter
	meta            Meta
	conversational  bool
}

// NewSession constructs a session with references to shared LLM & memory, but isolated state.
func NewSession(ctx context.Context, stepInfoEmitter StepInfoEmitter, llm *LLM, mem Memory, knowledgeBlock *MemoryBlock, ag *Agent, messageHistory *MessageList, meta Meta, conversational bool) *Session { //nolint:revive
	ctx, cancel := context.WithCancel(ctx)
	ctx = context.WithValue(ctx, ContextKey("userID"), meta.UserID)
	ctx = context.WithValue(ctx, ContextKey("sessionID"), meta.SessionID)
	ctx = context.WithValue(ctx, ContextKey("interactionID"), meta.InteractionID)
	ctx = context.WithValue(ctx, ContextKey("appID"), meta.AppID)
	ctx = context.WithValue(ctx, ContextKey("extra"), meta.Extra)
	s := &Session{
		ctx:       ctx,
		cancel:    cancel,
		closeOnce: sync.Once{},

		inUserChannel:  make(chan string),
		outUserChannel: make(chan Response),

		llm:             llm,
		memory:          mem,
		knowledgeBlock:  knowledgeBlock,
		agent:           ag,
		messageHistory:  messageHistory,
		stepInfoEmitter: stepInfoEmitter,
		meta:            meta,
		conversational:  conversational,
	}
	go s.run()
	return s
}

// In processes incoming user messages. Could queue or immediately handle them.
func (s *Session) In(userMessage string) {
	s.inUserChannel <- userMessage
}

// Out retrieves the next message from the output channel, blocking until a message is available.
func (s *Session) Out() Response {
	response := <-s.outUserChannel
	return response
}

// Close ends the session lifecycle and releases any resources if needed.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		close(s.inUserChannel)
	})
}

func (s *Session) GetMessageHistory() *MessageList {
	return s.messageHistory
}

// run is the main loop for the session. It listens for user messages and process here. Although
// we don't support now, the idea is that session should support interactive mode which is why
// the input channel exists. Session should hold the control of how to route the messages to whichever agents
// when we support multiple agents.
// TODO - handle refusal everywhere
// TODO - handle other errors like network errors everywhere
func (s *Session) run() {
	log.Info().Str("session_id", s.meta.SessionID).Msg("Session started")
	defer log.Info().Str("session_id", s.meta.SessionID).Msg("Session ended")

	defer s.Close()
	select {
	case <-s.ctx.Done():
		log.Info().Str("session_id", s.meta.SessionID).Msg("Session ended (context done)")
		s.outUserChannel <- Response{Type: ResponseTypeEnd}
	case userMessage, ok := <-s.inUserChannel:
		if !ok {
			log.Error().Msg("Session input channel closed")
			s.outUserChannel <- Response{Type: ResponseTypeEnd}
			return
		}

		// Append user message to message history
		s.messageHistory.Add(UserMessage(userMessage))

		memoryBlock, err := s.memory.Retrieve(&s.meta)
		if err != nil {
			log.Error().Err(err).Msg("Error getting user info")
			return
		}

		// We use a two-channel approach to ensure proper message aggregation:
		// 1. An internal channel receives all agent responses
		// 2. These responses are processed sequentially in this goroutine
		// 3. Messages are aggregated here before being sent to storage
		// This prevents race conditions between aggregation and storage operations
		internalChannel := make(chan Response)
		var aggregatedResponse string

		// Ensure channel is closed when we're done with it
		defer close(internalChannel)

		go s.agent.Run(s.ctx, s.meta, s.llm, s.messageHistory, memoryBlock, s.knowledgeBlock, internalChannel, s.conversational)

		for response := range internalChannel {
			s.outUserChannel <- response
			if response.Type == ResponseTypePartialText {
				aggregatedResponse += response.Content

			}
			if response.Type == ResponseTypeEnd {
				break
			}
		}

		// Only add the assistant message to history if we actually have content
		// This prevents empty messages that can cause downstream API validation errors
		if aggregatedResponse != "" {
			s.messageHistory.Add(AssistantMessage(aggregatedResponse))
		}

		// Run method is done, send the final message
		s.outUserChannel <- Response{
			Type: ResponseTypeEnd,
		}
	}
}
