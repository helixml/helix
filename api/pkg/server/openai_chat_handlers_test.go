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
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
)

func TestOpenAIChatSuite(t *testing.T) {
	// NOTE: we want to make sure that both the users and
	// the runners can successfully trigger chat completions.
	suitCfgs := []struct {
		userID  string
		authCtx context.Context
	}{
		{"user_id", setRequestUser(context.Background(), types.User{
			ID:       "user_id",
			Email:    "foo@email.com",
			FullName: "Foo Bar",
		})},
		{"", setRequestUser(context.Background(), types.User{
			Token:     "runner_token",
			TokenType: types.TokenTypeRunner,
		})},
	}
	for _, cfg := range suitCfgs {
		testSuite := &OpenAIChatSuite{
			userID:  cfg.userID,
			authCtx: cfg.authCtx,
		}
		suite.Run(t, testSuite)
	}
}

type OpenAIChatSuite struct {
	suite.Suite

	ctrl            *gomock.Controller
	store           *store.MockStore
	pubsub          pubsub.PubSub
	openAiClient    *openai.MockClient
	rag             *rag.MockRAG
	providerManager *manager.MockProviderManager

	authCtx context.Context
	userID  string

	server *HelixAPIServer
}

func (suite *OpenAIChatSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())
	suite.ctrl = ctrl
	suite.store = store.NewMockStore(ctrl)
	// Add slot operation expectations for scheduler
	suite.store.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	suite.store.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	suite.store.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	suite.store.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	suite.store.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	suite.store.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	// Provider prefix lookup - return not found by default (model namespaces like "meta-llama" are not providers)
	suite.store.EXPECT().GetProviderEndpoint(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	// ListProviderEndpoints is called to check for model in user's custom providers
	suite.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{}, nil).AnyTimes()

	ps, err := pubsub.New(&config.ServerConfig{
		PubSub: config.PubSub{
			StoreDir: suite.T().TempDir(),
			Provider: string(pubsub.ProviderMemory),
		},
	})
	suite.NoError(err)

	suite.openAiClient = openai.NewMockClient(ctrl)
	suite.pubsub = ps

	filestoreMock := filestore.NewMockFileStore(ctrl)
	extractorMock := extract.NewMockExtractor(ctrl)
	suite.rag = rag.NewMockRAG(ctrl)

	cfg := &config.ServerConfig{}
	cfg.Tools.Enabled = false
	cfg.Inference.Provider = string(types.ProviderTogetherAI)

	providerManager := manager.NewMockProviderManager(ctrl)

	suite.providerManager = providerManager
	// It's called once during tool setup
	providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(suite.openAiClient, nil).Times(1)
	// ListProviders is called to check for model in global providers
	providerManager.EXPECT().ListProviders(gomock.Any(), gomock.Any()).Return([]types.Provider{}, nil).AnyTimes()

	runnerController, err := scheduler.NewRunnerController(context.Background(), &scheduler.RunnerControllerConfig{
		PubSub:        suite.pubsub,
		FS:            filestoreMock,
		HealthChecker: &scheduler.MockHealthChecker{},
	})
	suite.NoError(err)
	schedulerParams := &scheduler.Params{
		RunnerController: runnerController,
		Store:            suite.store,
	}
	scheduler, err := scheduler.NewScheduler(context.Background(), cfg, schedulerParams)
	suite.NoError(err)
	c, err := controller.NewController(context.Background(), controller.Options{
		Config:          cfg,
		Store:           suite.store,
		Janitor:         janitor.NewJanitor(config.Janitor{}),
		ProviderManager: providerManager,
		Filestore:       filestoreMock,
		Extractor:       extractorMock,
		RAG:             suite.rag,
		Scheduler:       scheduler,
		PubSub:          suite.pubsub,
	})
	suite.NoError(err)

	suite.server = &HelixAPIServer{
		Cfg:             cfg,
		pubsub:          suite.pubsub,
		Controller:      c,
		Store:           suite.store,
		providerManager: providerManager,
	}
}

func getTestOwnerID(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	user, ok := ctx.Value(userKey).(types.User)
	if !ok {
		return "", !ok
	}
	if user.TokenType == types.TokenTypeRunner {
		return openai.RunnerID, ok
	}
	return user.ID, ok
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

	ownerID, ok := getTestOwnerID(suite.authCtx)
	suite.Require().True(ok)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(suite.openAiClient, nil)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", req.Model)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("user", req.Messages[1].Role)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal(ownerID, vals.OwnerID)
			suite.True(strings.HasPrefix(vals.SessionID, "oai_"), "SessionID should start with 'oai_'")
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

	ownerID, ok := getTestOwnerID(suite.authCtx)
	suite.Require().True(ok)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	stream, writer, err := openai.NewOpenAIStreamingAdapter(oai.ChatCompletionRequest{})
	suite.Require().NoError(err)

	suite.providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(suite.openAiClient, nil)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

	suite.openAiClient.EXPECT().CreateChatCompletionStream(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ oai.ChatCompletionRequest) (*oai.ChatCompletionStream, error) {
			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal(ownerID, vals.OwnerID)
			suite.True(strings.HasPrefix(vals.SessionID, "oai_"), "SessionID should start with 'oai_'")
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

			err = writeChunk(writer, bts)
			suite.NoError(err)

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
	suite.store.EXPECT().GetAppWithTools(gomock.Any(), "app123").Return(app, nil).Times(1)
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

	ownerID, ok := getTestOwnerID(suite.authCtx)
	suite.Require().True(ok)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.providerManager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "togetherai",
		Owner:    suite.userID,
	}).Return(suite.openAiClient, nil)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", req.Model)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("you are very custom assistant", req.Messages[0].Content)

			suite.Equal("user", req.Messages[1].Role)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal(ownerID, vals.OwnerID)
			suite.True(strings.HasPrefix(vals.SessionID, "oai_"), "SessionID should start with 'oai_'")
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

func (suite *OpenAIChatSuite) TestChatCompletions_App_Blocking_Organization_Allowed() {

	app := &types.App{
		OrganizationID: "org123",
		Owner:          "some_owner",
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

	// 1. Checking whether caller is org member
	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	// 2. Set up authorization mocks
	setupAuthorizationMocks(suite.store, app, suite.userID, []types.Resource{types.ResourceApplication}, []types.Action{types.ActionGet})

	suite.store.EXPECT().GetApp(gomock.Any(), "app123").Return(app, nil).Times(1)

	suite.store.EXPECT().GetAppWithTools(gomock.Any(), "app123").Return(app, nil).Times(1)
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

	ownerID, ok := getTestOwnerID(suite.authCtx)
	suite.Require().True(ok)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.providerManager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "togetherai",
		Owner:    app.OrganizationID,
	}).Return(suite.openAiClient, nil)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", req.Model)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("you are very custom assistant", req.Messages[0].Content)

			suite.Equal("user", req.Messages[1].Role)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal(ownerID, vals.OwnerID)
			suite.True(strings.HasPrefix(vals.SessionID, "oai_"), "SessionID should start with 'oai_'")
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

func (suite *OpenAIChatSuite) TestChatCompletions_App_Blocking_Organization_Denied_NotMember() {
	app := &types.App{
		OrganizationID: "org123",
		Owner:          "some_owner",
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

	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(nil, store.ErrNotFound)

	suite.store.EXPECT().GetApp(gomock.Any(), "app123").Return(app, nil).Times(1)

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

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusForbidden, rec.Code, rec.Body.String())
}

func (suite *OpenAIChatSuite) TestChatCompletions_App_Blocking_Organization_Denied_WrongPermissions() {
	app := &types.App{
		OrganizationID: "org123",
		Owner:          "some_owner",
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

	// 1. Checking whether caller is org member
	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	// 2. No app access
	setupAuthorizationMocks(suite.store, app, suite.userID, []types.Resource{types.ResourceApplication}, []types.Action{types.ActionList})

	suite.store.EXPECT().GetApp(gomock.Any(), "app123").Return(app, nil).Times(1)

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

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusForbidden, rec.Code, rec.Body.String())
}

func (suite *OpenAIChatSuite) TestChatCompletions_App_CustomProvider() {

	app := &types.App{
		Global: true,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						Provider:     "custom-endpoint",
						Model:        "custom-model",
						SystemPrompt: "you are very custom assistant",
					},
				},
			},
		},
	}

	// Override provider manager
	providerManager := manager.NewMockProviderManager(suite.ctrl)
	providerManager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "custom-endpoint",
		Owner:    suite.userID,
	}).Return(suite.openAiClient, nil).AnyTimes()

	suite.server.providerManager = providerManager

	suite.store.EXPECT().GetApp(gomock.Any(), "app123").Return(app, nil).Times(1)
	suite.store.EXPECT().GetAppWithTools(gomock.Any(), "app123").Return(app, nil).Times(1)
	suite.store.EXPECT().ListSecrets(gomock.Any(), &store.ListSecretsQuery{
		Owner: suite.userID,
	}).Return([]*types.Secret{}, nil)

	req, err := http.NewRequest("POST", "/v1/chat/completions?app_id=app123", bytes.NewBufferString(`{
		"model": "custom-model",
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

	ownerID, ok := getTestOwnerID(suite.authCtx)
	suite.Require().True(ok)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.providerManager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "custom-endpoint",
		Owner:    suite.userID,
	}).Return(suite.openAiClient, nil)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("custom-model", req.Model)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("you are very custom assistant", req.Messages[0].Content)

			suite.Equal("user", req.Messages[1].Role)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal(ownerID, vals.OwnerID)
			suite.True(strings.HasPrefix(vals.SessionID, "oai_"), "SessionID should start with 'oai_'")
			suite.Equal("n/a", vals.InteractionID)

			return oai.ChatCompletionResponse{
				Model: "custom-model",
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

	suite.Equal("custom-model", resp.Model)
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
	suite.store.EXPECT().GetAppWithTools(gomock.Any(), "app123").Return(app, nil).Times(1)
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

	ownerID, ok := getTestOwnerID(suite.authCtx)
	suite.Require().True(ok)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.providerManager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "helix",
		Owner:    suite.userID,
	}).Return(suite.openAiClient, nil)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("llama3:instruct", req.Model)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("you are very custom assistant", req.Messages[0].Content)

			suite.Equal("user", req.Messages[1].Role)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal(ownerID, vals.OwnerID)
			suite.True(strings.HasPrefix(vals.SessionID, "oai_"), "SessionID should start with 'oai_'")
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
	suite.store.EXPECT().GetAppWithTools(gomock.Any(), "app123").Return(app, nil).Times(1)
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
		DocumentIDList:    []string{},
		Pipeline:          types.TextPipeline,
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

	ownerID, ok := getTestOwnerID(suite.authCtx)
	suite.Require().True(ok)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	suite.providerManager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "togetherai",
		Owner:    suite.userID,
	}).Return(suite.openAiClient, nil)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

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
			suite.Equal(ownerID, vals.OwnerID)
			suite.True(strings.HasPrefix(vals.SessionID, "oai_"), "SessionID should start with 'oai_'")
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
		Owner:  suite.userID,
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
	suite.store.EXPECT().GetAppWithTools(gomock.Any(), "app123").Return(app, nil).Times(1)
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

	// Let's pack AppID into the auth context
	user := suite.authCtx.Value(userKey).(types.User)
	user.AppID = "app123"

	authCtx := setRequestUser(context.Background(), user)

	ownerID, ok := getTestOwnerID(authCtx)
	suite.Require().True(ok)

	req = req.WithContext(authCtx)

	rec := httptest.NewRecorder()

	suite.providerManager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "togetherai",
		Owner:    suite.userID,
	}).Return(suite.openAiClient, nil)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo", req.Model)

			suite.Require().Equal(2, len(req.Messages))

			suite.Equal("system", req.Messages[0].Role)
			suite.Equal("you are very custom assistant", req.Messages[0].Content)

			suite.Equal("user", req.Messages[1].Role)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal(ownerID, vals.OwnerID)
			suite.True(strings.HasPrefix(vals.SessionID, "oai_"), "SessionID should start with 'oai_'")
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
	suite.store.EXPECT().GetAppWithTools(gomock.Any(), "app123").Return(app, nil).Times(1)
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

	ownerID, ok := getTestOwnerID(suite.authCtx)
	suite.Require().True(ok)

	req = req.WithContext(suite.authCtx)

	rec := httptest.NewRecorder()

	stream, writer, err := openai.NewOpenAIStreamingAdapter(oai.ChatCompletionRequest{})
	suite.Require().NoError(err)

	suite.providerManager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "helix",
		Owner:    suite.userID,
	}).Return(suite.openAiClient, nil)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

	suite.openAiClient.EXPECT().CreateChatCompletionStream(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (*oai.ChatCompletionStream, error) {
			suite.Equal("llama3:instruct", req.Model)

			vals, ok := openai.GetContextValues(ctx)
			suite.True(ok)
			suite.Equal(ownerID, vals.OwnerID)
			suite.True(strings.HasPrefix(vals.SessionID, "oai_"), "SessionID should start with 'oai_'")
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

			err = writeChunk(writer, bts)
			suite.NoError(err)

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

func (suite *OpenAIChatSuite) TestChatCompletions_ProviderPrefix_GlobalProvider() {
	// Test using a global provider prefix (e.g., "openai/gpt-4")
	// Global providers don't need store lookup - they're checked via types.IsGlobalProvider()
	req, err := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{
		"model": "openai/gpt-4",
		"stream": false,
		"messages": [{"role": "user", "content": "hello"}]
	}`))
	suite.NoError(err)
	req = req.WithContext(suite.authCtx)
	rec := httptest.NewRecorder()

	suite.providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(suite.openAiClient, nil)
	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()
	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("gpt-4", req.Model) // Prefix stripped
			return oai.ChatCompletionResponse{
				Choices: []oai.ChatCompletionChoice{{Message: oai.ChatCompletionMessage{Content: "hi"}, FinishReason: "stop"}},
			}, nil
		})

	suite.server.createChatCompletion(rec, req)
	suite.Equal(http.StatusOK, rec.Code)
}

func (suite *OpenAIChatSuite) TestChatCompletions_ProviderPrefix_SystemProvider() {
	// Test system-owned provider (e.g., from DYNAMIC_PROVIDERS env var like "openrouter")
	// These require a store lookup since they're not in types.GlobalProviders

	// Create fresh mocks to avoid AnyTimes interference from SetupTest
	ctrl := gomock.NewController(suite.T())
	storeMock := store.NewMockStore(ctrl)

	storeMock.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
		Name:      "openrouter",
		Owner:     string(types.OwnerTypeSystem),
		OwnerType: types.OwnerTypeSystem,
	}).Return(&types.ProviderEndpoint{Name: "openrouter"}, nil)
	// ListProviderEndpoints is called to check for model in user's custom providers
	storeMock.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{}, nil).AnyTimes()

	originalStore := suite.server.Store
	suite.server.Store = storeMock
	defer func() { suite.server.Store = originalStore }()

	req, err := http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{
		"model": "openrouter/x-ai/grok-beta",
		"stream": false,
		"messages": [{"role": "user", "content": "hello"}]
	}`))
	suite.NoError(err)
	req = req.WithContext(suite.authCtx)
	rec := httptest.NewRecorder()

	suite.providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(suite.openAiClient, nil)
	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()
	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req oai.ChatCompletionRequest) (oai.ChatCompletionResponse, error) {
			suite.Equal("x-ai/grok-beta", req.Model) // Prefix stripped, nested path preserved
			return oai.ChatCompletionResponse{
				Choices: []oai.ChatCompletionChoice{{Message: oai.ChatCompletionMessage{Content: "hi"}, FinishReason: "stop"}},
			}, nil
		})

	suite.server.createChatCompletion(rec, req)
	suite.Equal(http.StatusOK, rec.Code)
}

func (suite *OpenAIChatSuite) TestChatCompletions_App_CustomQueryParams() {
	// Create test server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Equal("/custom", r.URL.Path)

		// Verify query parameters
		jobID := r.URL.Query().Get("job_id")
		suite.Equal("123", jobID)

		// Return successful response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(map[string]string{
			"result": "custom endpoint response",
		})
		suite.NoError(err)
	}))
	defer testServer.Close()

	tool, err := store.ConvertAPIToTool(types.AssistantAPI{
		URL: testServer.URL, // Use test server URL instead of hardcoded value
		Query: map[string]string{
			"job_id": "1234567890",
		},
		Schema: `{
			"openapi": "3.0.0",
			"info": {
				"title": "Custom API",
				"version": "1.0.0"
			},
			"paths": {
				"/custom": {
					"get": {
						"operationId": "custom",
						"summary": "Custom endpoint that requires job_id",
						"parameters": [
							{
								"name": "job_id",
								"in": "query",
								"required": true,
								"schema": {
									"type": "string"
								},
								"description": "The job ID to query"
							}
						],
						"responses": {
							"200": {
								"description": "Successful response",
								"content": {
									"application/json": {
										"schema": {
											"type": "object",
											"properties": {
												"result": {
													"type": "string"
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}`,
	})
	suite.NoError(err)
	app := &types.App{
		Global: true,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						SystemPrompt: "you are very custom assistant",
						Provider:     "custom-endpoint",
						Tools: []*types.Tool{
							tool,
						},
					},
				},
			},
		},
	}

	suite.store.EXPECT().GetApp(gomock.Any(), "app123").Return(app, nil).Times(1)
	suite.store.EXPECT().GetAppWithTools(gomock.Any(), "app123").Return(app, nil).Times(2)
	suite.store.EXPECT().ListSecrets(gomock.Any(), &store.ListSecretsQuery{
		Owner: suite.userID,
	}).Return([]*types.Secret{}, nil).Times(2)

	suite.store.EXPECT().CreateStepInfo(gomock.Any(), gomock.Any()).Times(4)

	req, err := http.NewRequest("POST", "/v1/chat/completions?app_id=app123&job_id=123", bytes.NewBufferString(`{
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

	isActionableResponse := tools.IsActionableResponse{
		NeedsTool:     tools.NeedsToolYes,
		Justification: "Test reason",
		API:           "custom",
	}
	isActionableResponseBts, _ := json.Marshal(isActionableResponse)

	suite.providerManager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "custom-endpoint",
		Owner:    suite.userID,
	}).Return(suite.openAiClient, nil).AnyTimes()

	suite.openAiClient.EXPECT().BillingEnabled().Return(true).AnyTimes()

	suite.openAiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).Return(oai.ChatCompletionResponse{
		Model: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
		Choices: []oai.ChatCompletionChoice{
			{
				Message: oai.ChatCompletionMessage{
					Role:    "assistant",
					Content: string(isActionableResponseBts),
				},
				FinishReason: "stop",
			},
		},
	}, nil).AnyTimes()

	// Begin the chat
	suite.server.createChatCompletion(rec, req)

	suite.Equal(http.StatusOK, rec.Code, rec.Body.String())

	var resp oai.ChatCompletionResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	suite.NoError(err)
}
