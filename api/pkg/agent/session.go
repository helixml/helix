// Package session provides the Session struct for per-conversation state,
// along with methods for handling user messages and producing agent outputs.
package agent

import (
	"context"
	"log/slog"
	"sync"
)

type Meta struct {
	UserID    string
	SessionID string
	Extra     map[string]string
}

// Session holds ephemeral conversation data & references to global resources.
type Session struct {
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once

	inUserChannel  chan string
	outUserChannel chan Response

	llm     *LLM
	memory  Memory
	agent   *Agent
	storage Storage

	meta Meta

	isConversational bool
	logger           *slog.Logger
}

// NewSession constructs a session with references to shared LLM & memory, but isolated state.
func NewSession(ctx context.Context, llm *LLM, mem Memory, ag *Agent, storage Storage, meta Meta) *Session {
	ctx, cancel := context.WithCancel(ctx)
	ctx = context.WithValue(ctx, ContextKey("userID"), meta.UserID)
	ctx = context.WithValue(ctx, ContextKey("sessionID"), meta.SessionID)
	ctx = context.WithValue(ctx, ContextKey("extra"), meta.Extra)
	s := &Session{
		ctx:       ctx,
		cancel:    cancel,
		closeOnce: sync.Once{},

		inUserChannel:  make(chan string),
		outUserChannel: make(chan Response),

		llm:     llm,
		memory:  mem,
		agent:   ag,
		storage: storage,

		meta: meta,

		logger: slog.Default(),

		isConversational: true,
	}
	go s.run()
	return s
}

func (s *Session) WithConversationDisabled() *Session {
	s.isConversational = false
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

// run is the main loop for the session. It listens for user messages and process here. Although
// we don't support now, the idea is that session should support interactive mode which is why
// the input channel exists. Session should hold the control of how to route the messages to whichever agents
// when we support multiple agents.
// TODO - handle refusal everywhere
// TODO - handle other errors like network errors everywhere
func (s *Session) run() {
	s.logger.Info("Session started", "sessionID", s.meta.SessionID)
	defer s.Close()
	select {
	case <-s.ctx.Done():
		s.outUserChannel <- Response{Type: ResponseTypeEnd}
	case userMessage, ok := <-s.inUserChannel:
		if !ok {
			s.logger.Error("Session input channel closed")
			s.outUserChannel <- Response{Type: ResponseTypeEnd}
			return
		}
		err := s.storage.CreateConversation(s.meta, userMessage)
		if err != nil {
			s.logger.Error("Error creating conversation", "error", err)
		}

		// Prepare session message history and validate state
		messageHistory, err := CompileConversationHistory(s.meta, s.storage)
		if err != nil {
			s.logger.Error("Error compiling conversation history", "error", err)
			return
		}

		memoryBlock, err := s.memory.Retrieve(&s.meta)
		if err != nil {
			s.logger.Error("Error getting user info", "error", err)
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

		go s.agent.Run(s.ctx, s.meta, s.llm, messageHistory, memoryBlock, internalChannel, s.isConversational)

		for response := range internalChannel {
			s.outUserChannel <- response
			if response.Type == ResponseTypePartialText {
				aggregatedResponse += response.Content
			}
			if response.Type == ResponseTypeEnd {
				break
			}
		}

		// Finish the conversation in the store with the fully aggregated response
		err = s.storage.FinishConversation(s.meta, aggregatedResponse)
		if err != nil {
			s.logger.Error("Error finishing conversation", "error", err)
		}

		// Run method is done, send the final message
		s.outUserChannel <- Response{
			Type: ResponseTypeEnd,
		}
	}
}
