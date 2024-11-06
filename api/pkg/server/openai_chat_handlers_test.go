package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	oai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
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
)

func TestOpenAIChatSuite(t *testing.T) {
	suite.Run(t, new(OpenAIChatSuite))
}

type OpenAIChatSuite struct {
	suite.Suite

	store        *store.MockStore
	pubsub       pubsub.PubSub
	openAiClient *openai.MockClient
	rag          *rag.MockRAG

	authCtx context.Context
	userID  string

	server *HelixAPIServer
}

func (suite *OpenAIChatSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())

	suite.store = store.NewMockStore(ctrl)
	ps, err := pubsub.New(suite.T().TempDir())
	suite.NoError(err)

	suite.openAiClient = openai.NewMockClient(ctrl)
	suite.pubsub = ps

	filestoreMock := filestore.NewMockFileStore(ctrl)
	extractorMock := extract.NewMockExtractor(ctrl)
	suite.rag = rag.NewMockRAG(ctrl)

	suite.userID = "user_id"
	suite.authCtx = setRequestUser(context.Background(), types.User{
		ID:       suite.userID,
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	})

	cfg := &config.ServerConfig{}
	cfg.Tools.Enabled = false
	cfg.Inference.Provider = types.ProviderTogetherAI

	providerManager := manager.NewMockProviderManager(ctrl)
	providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(suite.openAiClient, nil).AnyTimes()

	c, err := controller.NewController(context.Background(), controller.ControllerOptions{
		Config:          cfg,
		Store:           suite.store,
		Janitor:         janitor.NewJanitor(config.Janitor{}),
		ProviderManager: providerManager,
		Filestore:       filestoreMock,
		Extractor:       extractorMock,
		RAG:             suite.rag,
		Scheduler:       scheduler.NewScheduler(context.Background(), cfg, nil),
	})
	suite.NoError(err)

	suite.server = &HelixAPIServer{
		Cfg:        cfg,
		pubsub:     suite.pubsub,
		Controller: c,
		Store:      suite.store,
	}
}

func (suite *OpenAIChatSuite) TestChatCompletions_Basic_Blocking() {

	req, err := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{
		"model": "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
		"stream": false,
		"messages": [
			{
				"role": "system",
				"content": "You are a helpful assistant."
			},
			{
				"role": "user",
				"content": "tell me about oceans!"
			}
		]
	}`))
	suite.NoError(err)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", req.Model)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("user", req.Messages[1].Role)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal("user_id", vals.OwnerID)
			suite.Equal("n/a", vals.SessionID)
			suite.Equal("n/a", vals.InteractionID)

			return oai.ChatCompletionResponse{
				Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
				Choices: []oai.ChatCompletionChoice{
					{
						Message: oai.ChatCompletionMessage{
							Role:    "assistant",
							Content: "**model-result**",
						},
						FinishReason: "stop",
					},
				},
			}, nil
		})

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusOK, rec.Code)

	var resp oai.ChatCompletionResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	suite.NoError(err)

	suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", resp.Model)
	require.Equal(suite.T(), 1, len(resp.Choices), "should contain 1 choice")
	suite.Equal(oai.FinishReasonStop, resp.Choices[0].FinishReason)
	suite.Equal("assistant", resp.Choices[0].Message.Role)
	suite.Equal("**model-result**", resp.Choices[0].Message.Content)
}

func (suite *OpenAIChatSuite) TestChatCompletions_Streaming() {

	req, err := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{
		"model": "mistralai/Mistral-7B-Instruct-v0.1",
		"stream": true,
		"messages": [
			{
				"role": "system",
				"content": "You are a helpful assistant."
			},
			{
				"role": "user",
				"content": "tell me about oceans!"
			}
		]
	}`))
	suite.NoError(err)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	stream, writer, err := openai.NewOpenAIStreamingAdapter(oai.ChatCompletionRequest{})
	suite.Require().NoError(err)

	suite.openAiClient.EXPECT().CreateChatCompletionStream(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (*oai.ChatCompletionStream, error) {
			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal("user_id", vals.OwnerID)
			suite.Equal("n/a", vals.SessionID)
			suite.Equal("n/a", vals.InteractionID)

			return stream, nil
		})

	go func() {
		for i := 0; i < 3; i++ {
			// Create a chat completion chunk and encode it to json
			chunk := oai.ChatCompletionStreamResponse{
				ID:     "chatcmpl-123",
				Object: "chat.completion.chunk",
				Model:  "mistralai/Mistral-7B-Instruct-v0.1",
				Choices: []oai.ChatCompletionStreamChoice{
					{
						Delta: oai.ChatCompletionStreamChoiceDelta{
							Content: fmt.Sprintf("msg-%d", i),
						},
					},
				},
			}

			if i == 0 {
				chunk.Choices[0].Delta.Role = "assistant"
			}

			if i == 2 {
				chunk.Choices[0].FinishReason = "stop"
			}

			bts, err := json.Marshal(chunk)
			suite.NoError(err)

			writeChunk(writer, bts)

			// _, err = writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(bts))))
			// suite.NoError(err)
		}

		_, err = writer.Write([]byte("[DONE]"))
		suite.NoError(err)

		writer.Close()
	}()

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusOK, rec.Code)

	// validate headers
	suite.Equal("text/event-stream", rec.Header().Get("Content-Type"))
	suite.Equal("no-cache", rec.Header().Get("Cache-Control"))
	suite.Equal("keep-alive", rec.Header().Get("Connection"))

	var (
		startFound = false
		stopFound  = false
		fullResp   string
	)

	// Read chunks
	scanner := bufio.NewScanner(rec.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			jsonData := line[6:] // Remove "data: " prefix
			if jsonData == "[DONE]" {
				break
			}

			var data oai.ChatCompletionStreamResponse
			err := json.Unmarshal([]byte(jsonData), &data)
			suite.NoError(err)

			suite.Equal("mistralai/Mistral-7B-Instruct-v0.1", data.Model)
			suite.Equal(1, len(data.Choices))

			suite.Equal("chat.completion.chunk", data.Object)

			fullResp = fullResp + data.Choices[0].Delta.Content

			if data.Choices[0].Delta.Role == "assistant" {
				startFound = true
			}

			if data.Choices[0].FinishReason == "stop" {
				stopFound = true
			}

			switch data.Choices[0].Delta.Content {
			case "msg-0":
				suite.Equal("msg-0", data.Choices[0].Delta.Content)
			case "msg-1":
				suite.Equal("msg-1", data.Choices[0].Delta.Content)
			case "msg-2":
				suite.Equal("msg-2", data.Choices[0].Delta.Content)
			case "":

			default:
				suite.T().Fatalf("unexpected message: %s", data.Choices[0].Delta.Content)
			}
		}
	}

	suite.T().Log(fullResp)

	suite.True(startFound, "start chunk not found")
	suite.True(stopFound, "stop chunk not found")
}

func (suite *OpenAIChatSuite) TestChatCompletions_App_Blocking() {

	app := &types.App{
		Global: true,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						SystemPrompt: "you are very custom assistant",
					},
				},
			},
		},
	}

	suite.store.EXPECT().GetApp(gomock.Any(), "app123").Return(app, nil).Times(1)
	suite.store.EXPECT().ListSecrets(gomock.Any(), &store.ListSecretsQuery{
		Owner: suite.userID,
	}).Return([]*types.Secret{}, nil)

	req, err := http.NewRequest("POST", "/v1/chat/completions?app_id=app123", bytes.NewBufferString(`{
		"model": "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
		"stream": false,
		"messages": [
			{
				"role": "system",
				"content": "You are a helpful assistant."
			},
			{
				"role": "user",
				"content": "tell me about oceans!"
			}
		]
	}`))
	suite.NoError(err)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", req.Model)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("you are very custom assistant", req.Messages[0].Content)

			suite.Equal("user", req.Messages[1].Role)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal("user_id", vals.OwnerID)
			suite.Equal("n/a", vals.SessionID)
			suite.Equal("n/a", vals.InteractionID)

			return oai.ChatCompletionResponse{
				Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
				Choices: []oai.ChatCompletionChoice{
					{
						Message: oai.ChatCompletionMessage{
							Role:    "assistant",
							Content: "**model-result**",
						},
						FinishReason: "stop",
					},
				},
			}, nil
		})

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusOK, rec.Code, rec.Body.String())

	var resp oai.ChatCompletionResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	suite.NoError(err)

	suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", resp.Model)
	require.Equal(suite.T(), 1, len(resp.Choices), "should contain 1 choice")
	suite.Equal(oai.FinishReasonStop, resp.Choices[0].FinishReason)
	suite.Equal("assistant", resp.Choices[0].Message.Role)
	suite.Equal("**model-result**", resp.Choices[0].Message.Content)
}

func (suite *OpenAIChatSuite) TestChatCompletions_App_HelixModel() {
	suite.server.Cfg.Inference.Provider = "helix"

	app := &types.App{
		Global: true,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						SystemPrompt: "you are very custom assistant",
						Model:        "helix-3.5",
					},
				},
			},
		},
	}

	suite.store.EXPECT().GetApp(gomock.Any(), "app123").Return(app, nil).Times(1)
	suite.store.EXPECT().ListSecrets(gomock.Any(), &store.ListSecretsQuery{
		Owner: suite.userID,
	}).Return([]*types.Secret{}, nil)

	req, err := http.NewRequest("POST", "/v1/chat/completions?app_id=app123", bytes.NewBufferString(`{		
		"stream": false,
		"messages": [
			{
				"role": "system",
				"content": "You are a helpful assistant."
			},
			{
				"role": "user",
				"content": "tell me about oceans!"
			}
		]
	}`))
	suite.NoError(err)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("llama3:instruct", req.Model)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("you are very custom assistant", req.Messages[0].Content)

			suite.Equal("user", req.Messages[1].Role)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal("user_id", vals.OwnerID)
			suite.Equal("n/a", vals.SessionID)
			suite.Equal("n/a", vals.InteractionID)

			return oai.ChatCompletionResponse{
				Model: "llama3:instruct",
				Choices: []oai.ChatCompletionChoice{
					{
						Message: oai.ChatCompletionMessage{
							Role:    "assistant",
							Content: "**model-result**",
						},
						FinishReason: "stop",
					},
				},
			}, nil
		})

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusOK, rec.Code, rec.Body.String())

	var resp oai.ChatCompletionResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	suite.NoError(err)

	suite.Equal("llama3:instruct", resp.Model)
	require.Equal(suite.T(), 1, len(resp.Choices), "should contain 1 choice")
	suite.Equal(oai.FinishReasonStop, resp.Choices[0].FinishReason)
	suite.Equal("assistant", resp.Choices[0].Message.Role)
	suite.Equal("**model-result**", resp.Choices[0].Message.Content)
}

func (suite *OpenAIChatSuite) TestChatCompletions_AppRag_Blocking() {

	const (
		ragSourceID = "rag-source-id"
	)

	app := &types.App{
		Global: true,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						SystemPrompt: "you are very custom assistant",
						RAGSourceID:  ragSourceID,
					},
				},
			},
		},
	}

	suite.store.EXPECT().GetApp(gomock.Any(), "app123").Return(app, nil).Times(1)
	suite.store.EXPECT().ListSecrets(gomock.Any(), &store.ListSecretsQuery{
		Owner: suite.userID,
	}).Return([]*types.Secret{}, nil)

	suite.store.EXPECT().GetDataEntity(gomock.Any(), ragSourceID).Return(&types.DataEntity{
		Owner: suite.userID,
		ID:    ragSourceID,
		Config: types.DataEntityConfig{
			RAGSettings: types.RAGSettings{
				Threshold:        40,
				DistanceFunction: "cosine",
				ResultsCount:     2,
			},
		},
	}, nil).Times(1)

	suite.rag.EXPECT().Query(gomock.Any(), &types.SessionRAGQuery{
		Prompt:            "tell me about oceans!",
		DataEntityID:      ragSourceID,
		DistanceThreshold: 40,
		DistanceFunction:  "cosine",
		MaxResults:        2,
	}).Return([]*types.SessionRAGResult{
		{
			Content: "This is a test RAG source 1",
		},
		{
			Content: "This is a test RAG source 2",
		},
	}, nil)

	req, err := http.NewRequest("POST", "/v1/chat/completions?app_id=app123", bytes.NewBufferString(`{
		"model": "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
		"stream": false,
		"messages": [
			{
				"role": "system",
				"content": "You are a helpful assistant."
			},
			{
				"role": "user",
				"content": "tell me about oceans!"
			}
		]
	}`))
	suite.NoError(err)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			// Get the app id from the context
			appID, ok := openai.GetContextAppID(ctx)
			suite.True(ok)
			suite.Equal("app123", appID)

			suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", req.Model)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("you are very custom assistant", req.Messages[0].Content)

			suite.Equal("user", req.Messages[1].Role)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal("user_id", vals.OwnerID)
			suite.Equal("n/a", vals.SessionID)
			suite.Equal("n/a", vals.InteractionID)

			suite.Contains(req.Messages[1].Content, "This is a test RAG source 1")
			suite.Contains(req.Messages[1].Content, "This is a test RAG source 2")

			return oai.ChatCompletionResponse{
				Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
				Choices: []oai.ChatCompletionChoice{
					{
						Message: oai.ChatCompletionMessage{
							Role:    "assistant",
							Content: "**model-result**",
						},
						FinishReason: "stop",
					},
				},
			}, nil
		})

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusOK, rec.Code, rec.Body.String())

	var resp oai.ChatCompletionResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	suite.NoError(err)

	suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", resp.Model)
	require.Equal(suite.T(), 1, len(resp.Choices), "should contain 1 choice")
	suite.Equal(oai.FinishReasonStop, resp.Choices[0].FinishReason)
	suite.Equal("assistant", resp.Choices[0].Message.Role)
	suite.Equal("**model-result**", resp.Choices[0].Message.Content)
}

// TestChatCompletions_AppFromAuth_Blocking test that simulates app id coming
// from the auth context
func (suite *OpenAIChatSuite) TestChatCompletions_AppFromAuth_Blocking() {

	app := &types.App{
		Global: true,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						SystemPrompt: "you are very custom assistant",
					},
				},
			},
		},
	}

	suite.store.EXPECT().GetApp(gomock.Any(), "app123").Return(app, nil).Times(2)
	suite.store.EXPECT().ListSecrets(gomock.Any(), &store.ListSecretsQuery{
		Owner: suite.userID,
	}).Return([]*types.Secret{}, nil)

	req, err := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{
		"model": "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
		"stream": false,
		"messages": [
			{
				"role": "system",
				"content": "You are a helpful assistant."
			},
			{
				"role": "user",
				"content": "tell me about oceans!"
			}
		]
	}`))
	suite.NoError(err)

	authCtx := setRequestUser(context.Background(), types.User{
		ID:       suite.userID,
		Email:    "foo@email.com",
		FullName: "Foo Bar",
		AppID:    "app123",
	})

	req = req.WithContext(authCtx)

	rec := httptest.NewRecorder()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", req.Model)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("you are very custom assistant", req.Messages[0].Content)

			suite.Equal("user", req.Messages[1].Role)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal("user_id", vals.OwnerID)
			suite.Equal("n/a", vals.SessionID)
			suite.Equal("n/a", vals.InteractionID)

			return oai.ChatCompletionResponse{
				Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
				Choices: []oai.ChatCompletionChoice{
					{
						Message: oai.ChatCompletionMessage{
							Role:    "assistant",
							Content: "**model-result**",
						},
						FinishReason: "stop",
					},
				},
			}, nil
		})

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusOK, rec.Code, rec.Body.String())

	var resp oai.ChatCompletionResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	suite.NoError(err)

	suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", resp.Model)
	require.Equal(suite.T(), 1, len(resp.Choices), "should contain 1 choice")
	suite.Equal(oai.FinishReasonStop, resp.Choices[0].FinishReason)
	suite.Equal("assistant", resp.Choices[0].Message.Role)
	suite.Equal("**model-result**", resp.Choices[0].Message.Content)
}

func (suite *OpenAIChatSuite) TestChatCompletions_App_Streaming() {
	suite.server.Cfg.Inference.Provider = "helix"

	app := &types.App{
		Global: true,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						SystemPrompt: "you are very custom assistant",
						Model:        "helix-3.5",
					},
				},
			},
		},
	}

	suite.store.EXPECT().GetApp(gomock.Any(), "app123").Return(app, nil).Times(1)
	suite.store.EXPECT().ListSecrets(gomock.Any(), &store.ListSecretsQuery{
		Owner: suite.userID,
	}).Return([]*types.Secret{}, nil)

	req, err := http.NewRequest("POST", "/v1/chat/completions?app_id=app123", bytes.NewBufferString(`{	
		"stream": true,
		"messages": [
			{
				"role": "system",
				"content": "You are a helpful assistant."
			},
			{
				"role": "user",
				"content": "tell me about oceans!"
			}
		]
	}`))
	suite.NoError(err)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	stream, writer, err := openai.NewOpenAIStreamingAdapter(oai.ChatCompletionRequest{})
	suite.Require().NoError(err)

	suite.openAiClient.EXPECT().CreateChatCompletionStream(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (*oai.ChatCompletionStream, error) {
			suite.Equal("llama3:instruct", req.Model)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal("user_id", vals.OwnerID)
			suite.Equal("n/a", vals.SessionID)
			suite.Equal("n/a", vals.InteractionID)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("you are very custom assistant", req.Messages[0].Content)

			return stream, nil
		})

	go func() {
		for i := 0; i < 3; i++ {
			// Create a chat completion chunk and encode it to json
			chunk := oai.ChatCompletionStreamResponse{
				ID:     "chatcmpl-123",
				Object: "chat.completion.chunk",
				Model:  "mistralai/Mistral-7B-Instruct-v0.1",
				Choices: []oai.ChatCompletionStreamChoice{
					{
						Delta: oai.ChatCompletionStreamChoiceDelta{
							Content: fmt.Sprintf("msg-%d", i),
						},
					},
				},
			}

			if i == 0 {
				chunk.Choices[0].Delta.Role = "assistant"
			}

			if i == 2 {
				chunk.Choices[0].FinishReason = "stop"
			}

			bts, err := json.Marshal(chunk)
			suite.NoError(err)

			writeChunk(writer, bts)

			// _, err = writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(bts))))
			// suite.NoError(err)
		}

		_, err = writer.Write([]byte("[DONE]"))
		suite.NoError(err)

		writer.Close()
	}()

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusOK, rec.Code)

	// validate headers
	suite.Equal("text/event-stream", rec.Header().Get("Content-Type"))
	suite.Equal("no-cache", rec.Header().Get("Cache-Control"))
	suite.Equal("keep-alive", rec.Header().Get("Connection"))

	var (
		startFound = false
		stopFound  = false
		fullResp   string
	)

	// Read chunks
	scanner := bufio.NewScanner(rec.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			jsonData := line[6:] // Remove "data: " prefix
			if jsonData == "[DONE]" {
				break
			}

			var data oai.ChatCompletionStreamResponse
			err := json.Unmarshal([]byte(jsonData), &data)
			suite.NoError(err)

			suite.Equal("mistralai/Mistral-7B-Instruct-v0.1", data.Model)
			suite.Equal(1, len(data.Choices))

			suite.Equal("chat.completion.chunk", data.Object)

			fullResp = fullResp + data.Choices[0].Delta.Content

			if data.Choices[0].Delta.Role == "assistant" {
				startFound = true
			}

			if data.Choices[0].FinishReason == "stop" {
				stopFound = true
			}

			switch data.Choices[0].Delta.Content {
			case "msg-0":
				suite.Equal("msg-0", data.Choices[0].Delta.Content)
			case "msg-1":
				suite.Equal("msg-1", data.Choices[0].Delta.Content)
			case "msg-2":
				suite.Equal("msg-2", data.Choices[0].Delta.Content)
			case "":

			default:
				suite.T().Fatalf("unexpected message: %s", data.Choices[0].Delta.Content)
			}
		}
	}

	suite.T().Log(fullResp)

	suite.True(startFound, "start chunk not found")
	suite.True(stopFound, "stop chunk not found")
}
