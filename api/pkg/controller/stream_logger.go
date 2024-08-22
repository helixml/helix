package controller

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"
)

const logCallTimeout = 5 * time.Second

// TODO: add an interface here?

// StreamLogger wraps the original ChatCompletionStream and accumulates the response
type StreamLogger struct {
	start           time.Time
	store           store.Store
	originalRequest *openai.ChatCompletionRequest
	originalStream  *openai.ChatCompletionStream
	accumulator     *strings.Builder
	mu              sync.Mutex
}

func NewStreamLogger(controller *Controller, store store.Store, request *openai.ChatCompletionRequest, stream *openai.ChatCompletionStream) *StreamLogger {
	return &StreamLogger{
		start:           time.Now(),
		store:           store,
		originalRequest: request,
		originalStream:  stream,
		accumulator:     &strings.Builder{},
	}
}

// Recv implements the ChatCompletionStream interface
func (ps *StreamLogger) Recv() (openai.ChatCompletionStreamResponse, error) {
	response, err := ps.originalStream.Recv()
	if err == nil {
		if len(response.Choices) > 0 {
			ps.mu.Lock()
			ps.accumulator.WriteString(response.Choices[0].Delta.Content)
			ps.mu.Unlock()
		}
	} else if err == io.EOF {
		// Stream is finished, log to database
		ps.logToDatabase()
	} else {
		log.Error().Msgf("Error receiving from stream: %v", err)
	}

	return response, err
}

// Close implements the ChatCompletionStream interface
func (ps *StreamLogger) Close() {
	ps.originalStream.Close()
}

// GetAccumulatedResponse returns the accumulated response
func (ps *StreamLogger) GetAccumulatedResponse() string {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.accumulator.String()
}

// logToDatabase logs the completed LLM call to the database
func (ps *StreamLogger) logToDatabase() {
	reqJSON, _ := json.Marshal(ps.originalRequest)
	respJSON, _ := json.Marshal(ps.GetAccumulatedResponse())
	llmCall := &types.LLMCall{
		SessionID:        "", // TODO
		InteractionID:    "", // TODO
		Model:            ps.originalRequest.Model,
		Step:             types.LLMCallStepRawStreaming,
		Request:          reqJSON,
		Response:         respJSON, // TODO include details from stream here too
		DurationMs:       time.Since(ps.start).Milliseconds(),
		PromptTokens:     int64(0), // TODO
		CompletionTokens: int64(0), // TODO
		TotalTokens:      int64(0), // TODO
	}
	ctx, cancel := context.WithTimeout(context.Background(), logCallTimeout)
	defer cancel()

	_, err := ps.store.CreateLLMCall(ctx, llmCall)
	if err != nil {
		log.Error().Err(err).Msg("failed to log LLM call")
	}
}
