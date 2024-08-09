package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	gomock "github.com/golang/mock/gomock"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestHelixClientTestSuite(t *testing.T) {
	suite.Run(t, new(HelixClientTestSuite))
}

type HelixClientTestSuite struct {
	ctx context.Context
	suite.Suite
	ctrl   *gomock.Controller
	pubsub pubsub.PubSub

	srv *InternalHelixServer
}

func (suite *HelixClientTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.ctrl = gomock.NewController(suite.T())

	pubsub, err := pubsub.NewInMemoryNats(suite.T().TempDir())
	suite.Require().NoError(err)

	suite.pubsub = pubsub

	cfg := &config.ServerConfig{}

	suite.srv = NewInternalHelixServer(cfg, pubsub)
}

func (suite *HelixClientTestSuite) Test_CreateChatCompletion_ValidateQueue() {
	var (
		ownerID       = "owner1"
		sessionID     = "session1"
		interactionID = "interaction1"
	)

	go func() {
		ctx, cancel := context.WithTimeout(suite.ctx, 100*time.Millisecond)
		defer cancel()

		ctx = SetContextValues(ctx, ownerID, sessionID, interactionID)
		_, _ = suite.srv.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:  types.Model_Ollama_Llama3_8b.String(),
			Stream: false,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    "system",
					Content: "system prompt",
				},
				{
					Role:    "user",
					Content: "user prompt",
				},
			},
		})
	}()

	// Request should be in the queue
	time.Sleep(50 * time.Millisecond)

	suite.srv.queueMu.Lock()
	defer suite.srv.queueMu.Unlock()

	suite.Len(suite.srv.queue, 1)

	req := suite.srv.queue[0]

	suite.Equal(ownerID, req.OwnerID)
	suite.Equal(sessionID, req.SessionID)
	suite.Equal(interactionID, req.InteractionID)

	suite.Len(req.Request.Messages, 2)

	suite.Equal("system", req.Request.Messages[0].Role)
	suite.Equal("system prompt", req.Request.Messages[0].Content)

	suite.Equal("user", req.Request.Messages[1].Role)
	suite.Equal("user prompt", req.Request.Messages[1].Content)
}

func (suite *HelixClientTestSuite) Test_CreateChatCompletion_Response() {
	var (
		ownerID       = "owner1"
		sessionID     = "session1"
		interactionID = "interaction1"
	)

	// Fake running will pick up our request and send a response
	go startFakeRunner(suite.T(), suite.srv, []*types.RunnerLLMInferenceResponse{
		{
			OwnerID:       ownerID,
			SessionID:     sessionID,
			InteractionID: interactionID,
			Response: &openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Content: "Hello, world!",
						},
					},
				},
				Usage: openai.Usage{
					PromptTokens:     5,
					CompletionTokens: 12,
					TotalTokens:      17,
				},
			},
		},
	})

	ctx := SetContextValues(suite.ctx, ownerID, sessionID, interactionID)

	resp, err := suite.srv.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    types.Model_Ollama_Llama3_8b.String(),
		Stream:   false,
		Messages: []openai.ChatCompletionMessage{},
	})
	suite.NoError(err)

	suite.Equal("Hello, world!", resp.Choices[0].Message.Content)
}

func (suite *HelixClientTestSuite) Test_CreateChatCompletion_ErrorResponse() {
	var (
		ownerID       = "owner1"
		sessionID     = "session1"
		interactionID = "interaction1"
	)

	// Fake running will pick up our request and send a response
	go startFakeRunner(suite.T(), suite.srv, []*types.RunnerLLMInferenceResponse{
		{
			OwnerID:       ownerID,
			SessionID:     sessionID,
			InteractionID: interactionID,
			Error:         "too many tokens",
		},
	})

	ctx := SetContextValues(suite.ctx, ownerID, sessionID, interactionID)

	_, err := suite.srv.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    types.Model_Ollama_Llama3_8b.String(),
		Stream:   false,
		Messages: []openai.ChatCompletionMessage{},
	})
	suite.Error(err)
	suite.Contains(err.Error(), "too many tokens")
}

func (suite *HelixClientTestSuite) Test_CreateChatCompletion_StreamingResponse() {
	var (
		ownerID       = "owner1"
		sessionID     = "session1"
		interactionID = "interaction1"
	)

	// Fake running will pick up our request and send a response
	go startFakeRunner(suite.T(), suite.srv, []*types.RunnerLLMInferenceResponse{
		{
			OwnerID:       ownerID,
			SessionID:     sessionID,
			InteractionID: interactionID,
			StreamResponse: &openai.ChatCompletionStreamResponse{
				Choices: []openai.ChatCompletionStreamChoice{
					{
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Content: "One,",
						},
					},
				},
			},
		},
		{
			OwnerID:       ownerID,
			SessionID:     sessionID,
			InteractionID: interactionID,
			StreamResponse: &openai.ChatCompletionStreamResponse{
				Choices: []openai.ChatCompletionStreamChoice{
					{
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Content: "Two,",
						},
					},
				},
			},
		},
		{
			OwnerID:       ownerID,
			SessionID:     sessionID,
			InteractionID: interactionID,
			StreamResponse: &openai.ChatCompletionStreamResponse{
				Choices: []openai.ChatCompletionStreamChoice{
					{
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Content: "Three.",
						},
					},
				},
			},
			Done: true,
		},
	})

	ctx := SetContextValues(suite.ctx, ownerID, sessionID, interactionID)

	stream, err := suite.srv.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    types.Model_Ollama_Llama3_8b.String(),
		Stream:   true,
		Messages: []openai.ChatCompletionMessage{},
	})
	suite.NoError(err)

	defer stream.Close()

	var resp string

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		suite.NoError(err)

		resp += response.Choices[0].Delta.Content
	}

	suite.Equal("One,Two,Three.", resp)
}

// startFakeRunner starts polling the queue for requests and sends responses. Exits once context
// is done
func startFakeRunner(t *testing.T, srv *InternalHelixServer, responses []*types.RunnerLLMInferenceResponse) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	t.Cleanup(func() {
		cancel()
	})

	for {
		select {
		case <-ctx.Done():
			return
		default:
			req, err := srv.GetNextLLMInferenceRequest(ctx, types.InferenceRequestFilter{}, "runner1")
			require.NoError(t, err)

			if req == nil {
				time.Sleep(200 * time.Millisecond)
				continue
			}

			t.Logf("sending response for request %s (owner: %s | session: %s)", req.RequestID, req.OwnerID, req.SessionID)

			for _, resp := range responses {
				bts, err := json.Marshal(resp)
				require.NoError(t, err)

				err = srv.pubsub.Publish(ctx, pubsub.GetRunnerResponsesQueue(req.OwnerID, req.RequestID), bts)
				require.NoError(t, err)
			}
			fmt.Println("all responses sent")
			t.Log("all responses sent")

		}
	}
}
