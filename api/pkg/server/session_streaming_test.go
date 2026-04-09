package server

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	oai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"net/http/httptest"
)

func TestSessionStreamingSuite(t *testing.T) {
	suite.Run(t, new(SessionStreamingSuite))
}

type SessionStreamingSuite struct {
	suite.Suite

	ctrl            *gomock.Controller
	store           *store.MockStore
	pubsub          pubsub.PubSub
	openAiClient    *openai.MockClient
	rag             *rag.MockRAG
	providerManager *manager.MockProviderManager

	server *HelixAPIServer
}

func (s *SessionStreamingSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())
	s.ctrl = ctrl
	s.store = store.NewMockStore(ctrl)

	// Standard mock expectations
	s.store.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	s.store.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.store.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.store.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	s.store.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	s.store.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	s.store.EXPECT().GetProviderEndpoint(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{}, nil).AnyTimes()

	ps, err := pubsub.New(&config.ServerConfig{
		PubSub: config.PubSub{
			StoreDir: s.T().TempDir(),
			Provider: string(pubsub.ProviderMemory),
		},
	})
	s.NoError(err)

	s.openAiClient = openai.NewMockClient(ctrl)
	s.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()
	s.pubsub = ps
	s.rag = rag.NewMockRAG(ctrl)

	filestoreMock := filestore.NewMockFileStore(ctrl)
	extractorMock := extract.NewMockExtractor(ctrl)

	cfg := &config.ServerConfig{}
	cfg.Tools.Enabled = false
	cfg.Inference.Provider = string(types.ProviderTogetherAI)

	providerManager := manager.NewMockProviderManager(ctrl)
	s.providerManager = providerManager
	providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil).Times(1)
	providerManager.EXPECT().ListProviders(gomock.Any(), gomock.Any()).Return([]types.Provider{}, nil).AnyTimes()

	runnerController, err := scheduler.NewRunnerController(context.Background(), &scheduler.RunnerControllerConfig{
		PubSub:        s.pubsub,
		FS:            filestoreMock,
		HealthChecker: &scheduler.MockHealthChecker{},
	})
	s.NoError(err)
	schedulerParams := &scheduler.Params{
		RunnerController: runnerController,
		Store:            s.store,
	}
	sched, err := scheduler.NewScheduler(context.Background(), cfg, schedulerParams)
	s.NoError(err)

	c, err := controller.NewController(context.Background(), controller.Options{
		Config:          cfg,
		Store:           s.store,
		Janitor:         janitor.NewJanitor(config.Janitor{}),
		ProviderManager: providerManager,
		Filestore:       filestoreMock,
		Extractor:       extractorMock,
		RAG:             s.rag,
		Scheduler:       sched,
		PubSub:          s.pubsub,
	})
	s.NoError(err)

	s.server = &HelixAPIServer{
		Cfg:             cfg,
		pubsub:          s.pubsub,
		Controller:      c,
		Store:           s.store,
		providerManager: providerManager,
	}
}

// createMockStream creates a streaming adapter and writes chunks to it in a goroutine.
// Returns the stream that can be returned from a mocked CreateChatCompletionStream.
func (s *SessionStreamingSuite) createMockStream(chunks ...string) *oai.ChatCompletionStream {
	stream, writer, err := openai.NewOpenAIStreamingAdapter(oai.ChatCompletionRequest{})
	s.Require().NoError(err)

	go func() {
		for i, content := range chunks {
			chunk := oai.ChatCompletionStreamResponse{
				Object: "chat.completion.chunk",
				Model:  "test-model",
				Choices: []oai.ChatCompletionStreamChoice{
					{
						Index: 0,
						Delta: oai.ChatCompletionStreamChoiceDelta{
							Content: content,
						},
					},
				},
			}
			if i == 0 {
				chunk.Choices[0].Delta.Role = "assistant"
			}
			if i == len(chunks)-1 {
				chunk.Choices[0].FinishReason = "stop"
			}

			bts, _ := json.Marshal(chunk)
			_ = writeChunk(writer, bts)
		}
		_, _ = writer.Write([]byte("[DONE]"))
		writer.Close()
	}()

	return stream
}

// setupStreamExpectations sets up the common mock expectations for a streaming test.
func (s *SessionStreamingSuite) setupStreamExpectations(session *types.Session, interaction *types.Interaction) {
	s.store.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil).AnyTimes()
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(session, nil).AnyTimes()
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return([]*types.Interaction{interaction}, int64(1), nil).AnyTimes()
}

// TestStreamingSession_SSEHeaders verifies that the streaming session handler
// sets the correct SSE headers including X-Accel-Buffering for reverse proxy support.
func (s *SessionStreamingSuite) TestStreamingSession_SSEHeaders() {
	session := &types.Session{ID: "ses_test_headers", Owner: "user_id"}
	interaction := &types.Interaction{ID: "int_test_headers", SessionID: session.ID, UserID: "user_id"}

	mockStream := s.createMockStream("Hello")
	s.providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().CreateChatCompletionStream(gomock.Any(), gomock.Any()).Return(mockStream, nil)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(interaction, nil)
	s.setupStreamExpectations(session, interaction)

	rec := httptest.NewRecorder()
	err := s.server.handleStreamingSession(
		context.Background(), &types.User{ID: "user_id"}, session, interaction,
		oai.ChatCompletionRequest{Model: "test-model", Stream: true, Messages: []oai.ChatCompletionMessage{{Role: "user", Content: "hello"}}},
		&controller.ChatCompletionOptions{}, rec,
	)
	s.NoError(err)

	s.Equal("text/event-stream", rec.Header().Get("Content-Type"))
	s.Equal("no-cache", rec.Header().Get("Cache-Control"))
	s.Equal("keep-alive", rec.Header().Get("Connection"))
	s.Equal("int_test_headers", rec.Header().Get("X-Interaction-ID"))
}

// TestStreamingSession_InitialChunkContainsSessionID verifies that the first SSE chunk
// contains the session ID so the frontend can identify which session to stream into.
func (s *SessionStreamingSuite) TestStreamingSession_InitialChunkContainsSessionID() {
	session := &types.Session{ID: "ses_test_initial", Owner: "user_id"}
	interaction := &types.Interaction{ID: "int_test_initial", SessionID: session.ID, UserID: "user_id"}

	mockStream := s.createMockStream("Hi")
	s.providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().CreateChatCompletionStream(gomock.Any(), gomock.Any()).Return(mockStream, nil)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(interaction, nil)
	s.setupStreamExpectations(session, interaction)

	rec := httptest.NewRecorder()
	err := s.server.handleStreamingSession(
		context.Background(), &types.User{ID: "user_id"}, session, interaction,
		oai.ChatCompletionRequest{Model: "test-model", Stream: true, Messages: []oai.ChatCompletionMessage{{Role: "user", Content: "hi"}}},
		&controller.ChatCompletionOptions{}, rec,
	)
	s.NoError(err)

	chunks := parseSSEChunks(rec.Body.String())
	s.Require().NotEmpty(chunks, "expected at least one SSE chunk")

	var firstChunk oai.ChatCompletionStreamResponse
	err = json.Unmarshal([]byte(chunks[0]), &firstChunk)
	s.NoError(err)
	s.Equal("ses_test_initial", firstChunk.ID, "first chunk should contain the session ID")
}

// TestStreamingSession_StreamsTokensInSSEFormat verifies that LLM tokens are streamed
// as SSE data chunks in the correct format.
func (s *SessionStreamingSuite) TestStreamingSession_StreamsTokensInSSEFormat() {
	session := &types.Session{ID: "ses_test_tokens", Owner: "user_id"}
	interaction := &types.Interaction{ID: "int_test_tokens", SessionID: session.ID, UserID: "user_id"}

	mockStream := s.createMockStream("Hello", " ", "World")
	s.providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().CreateChatCompletionStream(gomock.Any(), gomock.Any()).Return(mockStream, nil)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(interaction, nil)
	s.setupStreamExpectations(session, interaction)

	rec := httptest.NewRecorder()
	err := s.server.handleStreamingSession(
		context.Background(), &types.User{ID: "user_id"}, session, interaction,
		oai.ChatCompletionRequest{Model: "test-model", Stream: true, Messages: []oai.ChatCompletionMessage{{Role: "user", Content: "test"}}},
		&controller.ChatCompletionOptions{}, rec,
	)
	s.NoError(err)

	body := rec.Body.String()
	scanner := bufio.NewScanner(strings.NewReader(body))
	dataLines := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		s.True(strings.HasPrefix(line, "data: "), "all non-empty lines should be SSE format, got: %s", line)
		dataLines++
	}
	// At minimum: initial chunk + 3 content chunks + [DONE]
	s.GreaterOrEqual(dataLines, 5, "expected at least 5 data lines (initial + 3 content + done)")
}

// TestStreamingSession_DoneMarker verifies that [DONE] is sent at the end of the stream.
func (s *SessionStreamingSuite) TestStreamingSession_DoneMarker() {
	session := &types.Session{ID: "ses_test_done", Owner: "user_id"}
	interaction := &types.Interaction{ID: "int_test_done", SessionID: session.ID, UserID: "user_id"}

	mockStream := s.createMockStream("done test")
	s.providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().CreateChatCompletionStream(gomock.Any(), gomock.Any()).Return(mockStream, nil)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(interaction, nil)
	s.setupStreamExpectations(session, interaction)

	rec := httptest.NewRecorder()
	err := s.server.handleStreamingSession(
		context.Background(), &types.User{ID: "user_id"}, session, interaction,
		oai.ChatCompletionRequest{Model: "test-model", Stream: true, Messages: []oai.ChatCompletionMessage{{Role: "user", Content: "test done"}}},
		&controller.ChatCompletionOptions{}, rec,
	)
	s.NoError(err)

	body := rec.Body.String()
	s.Contains(body, "data: [DONE]", "stream should end with data: [DONE] marker")

	// [DONE] should be the last data line
	lines := strings.Split(strings.TrimSpace(body), "\n")
	lastDataLine := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "data: ") {
			lastDataLine = lines[i]
			break
		}
	}
	s.Equal("data: [DONE]", lastDataLine, "[DONE] should be the last data line")
}

// TestStreamingSession_InteractionUpdatedOnComplete verifies that the interaction
// state is set to complete with the full response after streaming finishes.
func (s *SessionStreamingSuite) TestStreamingSession_InteractionUpdatedOnComplete() {
	session := &types.Session{ID: "ses_test_complete", Owner: "user_id"}
	interaction := &types.Interaction{ID: "int_test_complete", SessionID: session.ID, UserID: "user_id"}

	mockStream := s.createMockStream("Hello", " ", "World")
	s.providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().CreateChatCompletionStream(gomock.Any(), gomock.Any()).Return(mockStream, nil)

	var capturedInteraction *types.Interaction
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			capturedInteraction = interaction
			return interaction, nil
		})
	s.setupStreamExpectations(session, interaction)

	rec := httptest.NewRecorder()
	err := s.server.handleStreamingSession(
		context.Background(), &types.User{ID: "user_id"}, session, interaction,
		oai.ChatCompletionRequest{Model: "test-model", Stream: true, Messages: []oai.ChatCompletionMessage{{Role: "user", Content: "test complete"}}},
		&controller.ChatCompletionOptions{}, rec,
	)
	s.NoError(err)

	s.Require().NotNil(capturedInteraction, "UpdateInteraction should have been called")
	s.Equal(types.InteractionStateComplete, capturedInteraction.State)
	s.Equal("Hello World", capturedInteraction.ResponseMessage)
	s.True(capturedInteraction.DurationMs >= 0, "duration should be non-negative")
	s.False(capturedInteraction.Completed.IsZero(), "completed timestamp should be set")
}

// TestStreamingSession_LLMErrorBeforeStream verifies that when the LLM returns an error
// before streaming starts, the interaction is set to error state and an error chunk is written.
func (s *SessionStreamingSuite) TestStreamingSession_LLMErrorBeforeStream() {
	session := &types.Session{ID: "ses_test_error", Owner: "user_id"}
	interaction := &types.Interaction{ID: "int_test_error", SessionID: session.ID, UserID: "user_id"}

	s.providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().CreateChatCompletionStream(gomock.Any(), gomock.Any()).
		Return(nil, io.ErrUnexpectedEOF)

	var capturedInteraction *types.Interaction
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			capturedInteraction = interaction
			return interaction, nil
		})
	s.store.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil).AnyTimes()
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return([]*types.Interaction{interaction}, int64(1), nil).AnyTimes()

	rec := httptest.NewRecorder()
	err := s.server.handleStreamingSession(
		context.Background(), &types.User{ID: "user_id"}, session, interaction,
		oai.ChatCompletionRequest{Model: "test-model", Stream: true, Messages: []oai.ChatCompletionMessage{{Role: "user", Content: "test error"}}},
		&controller.ChatCompletionOptions{}, rec,
	)
	s.NoError(err) // Handler returns nil, writes error to SSE

	s.Require().NotNil(capturedInteraction)
	s.Equal(types.InteractionStateError, capturedInteraction.State)
	s.NotEmpty(capturedInteraction.Error)

	body := rec.Body.String()
	s.Contains(body, `"error"`, "error should be written as SSE data")
}

// --- Helpers ---

// parseSSEChunks extracts the data payloads from SSE-formatted text, excluding [DONE].
func parseSSEChunks(body string) []string {
	var chunks []string
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data != "[DONE]" {
				chunks = append(chunks, data)
			}
		}
	}
	return chunks
}
