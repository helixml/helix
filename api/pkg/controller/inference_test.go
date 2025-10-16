package controller

import (
	"context"
	"reflect"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestControllerSuite(t *testing.T) {
	suite.Run(t, new(ControllerSuite))
}

type ControllerSuite struct {
	suite.Suite

	ctx context.Context

	store           *store.MockStore
	pubsub          pubsub.PubSub
	openAiClient    *oai.MockClient
	rag             *rag.MockRAG
	user            *types.User
	providerManager *manager.MockProviderManager

	controller *Controller
}

func (suite *ControllerSuite) SetupSuite() {
	ctrl := gomock.NewController(suite.T())

	suite.ctx = context.Background()
	suite.store = store.NewMockStore(ctrl)
	// Add slot operation expectations for scheduler
	suite.store.EXPECT().ListAllSlots(gomock.Any()).Return([]*types.RunnerSlot{}, nil).AnyTimes()
	suite.store.EXPECT().CreateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	suite.store.EXPECT().UpdateSlot(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	suite.store.EXPECT().DeleteSlot(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	suite.store.EXPECT().ListModels(gomock.Any(), gomock.Any()).Return([]*types.Model{}, nil).AnyTimes()
	suite.store.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()

	ps, err := pubsub.New(&config.ServerConfig{
		PubSub: config.PubSub{
			Provider: string(pubsub.ProviderMemory),
		},
	})
	suite.NoError(err)

	suite.pubsub = ps

	suite.openAiClient = oai.NewMockClient(ctrl)
	suite.providerManager = manager.NewMockProviderManager(ctrl)

	suite.providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(suite.openAiClient, nil).AnyTimes()

	filestoreMock := filestore.NewMockFileStore(ctrl)
	extractorMock := extract.NewMockExtractor(ctrl)
	suite.rag = rag.NewMockRAG(ctrl)

	suite.user = &types.User{
		ID:       "user_id",
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	}

	cfg := &config.ServerConfig{}
	cfg.Tools.Enabled = false
	cfg.Inference.Provider = string(types.ProviderTogetherAI)

	runnerController, err := scheduler.NewRunnerController(suite.ctx, &scheduler.RunnerControllerConfig{
		PubSub:        suite.pubsub,
		FS:            filestoreMock,
		HealthChecker: &scheduler.MockHealthChecker{},
	})
	suite.NoError(err)
	schedulerParams := &scheduler.Params{
		RunnerController: runnerController,
		Store:            suite.store,
	}
	scheduler, err := scheduler.NewScheduler(suite.ctx, cfg, schedulerParams)
	suite.NoError(err)

	c, err := NewController(context.Background(), Options{
		Config:          cfg,
		Store:           suite.store,
		Janitor:         janitor.NewJanitor(config.Janitor{}),
		ProviderManager: suite.providerManager,
		Filestore:       filestoreMock,
		Extractor:       extractorMock,
		RAG:             suite.rag,
		Scheduler:       scheduler,
	})
	suite.NoError(err)

	suite.controller = c
}

func (suite *ControllerSuite) Test_BasicInference() {
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4TurboPreview,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: "Hello",
			},
		},
	}

	suite.openAiClient.EXPECT().BillingEnabled().Return(true)

	suite.openAiClient.EXPECT().CreateChatCompletion(suite.ctx, gomock.Any()).Return(openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: "Hello",
				},
			},
		},
	}, nil)

	resp, _, err := suite.controller.ChatCompletion(suite.ctx, suite.user, req, &ChatCompletionOptions{})
	suite.NoError(err)
	suite.Equal(&openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: "Hello",
				},
			},
		},
	}, resp)
}

func (suite *ControllerSuite) Test_BasicInference_WithBalanceCheck_Success() {
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4TurboPreview,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: "Hello",
			},
		},
	}

	suite.openAiClient.EXPECT().BillingEnabled().Return(true)

	suite.openAiClient.EXPECT().CreateChatCompletion(suite.ctx, gomock.Any()).Return(openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: "Hello",
				},
			},
		},
	}, nil)

	suite.controller.Options.Config.Stripe.BillingEnabled = true
	suite.controller.Options.Config.Stripe.MinimumInferenceBalance = 0.01

	suite.store.EXPECT().GetWalletByUser(suite.ctx, suite.user.ID).Return(&types.Wallet{
		Balance: 0.02,
	}, nil)

	resp, _, err := suite.controller.ChatCompletion(suite.ctx, suite.user, req, &ChatCompletionOptions{})
	suite.NoError(err)
	suite.Equal(&openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: "Hello",
				},
			},
		},
	}, resp)
}

func (suite *ControllerSuite) Test_BasicInference_WithBalanceCheck_InsufficientBalance() {
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4TurboPreview,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: "Hello",
			},
		},
	}

	suite.controller.Options.Config.Stripe.BillingEnabled = true
	suite.controller.Options.Config.Stripe.MinimumInferenceBalance = 0.01

	suite.openAiClient.EXPECT().BillingEnabled().Return(true)

	suite.store.EXPECT().GetWalletByUser(suite.ctx, suite.user.ID).Return(&types.Wallet{
		Balance: 0.009,
	}, nil)

	resp, _, err := suite.controller.ChatCompletion(suite.ctx, suite.user, req, &ChatCompletionOptions{})
	suite.Error(err)
	suite.Nil(resp)
}

func (suite *ControllerSuite) Test_BasicInference_WithBalanceCheck_Org_InsufficientBalance() {
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4TurboPreview,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: "Hello",
			},
		},
	}

	suite.controller.Options.Config.Stripe.BillingEnabled = true
	suite.controller.Options.Config.Stripe.MinimumInferenceBalance = 0.01

	suite.openAiClient.EXPECT().BillingEnabled().Return(true)

	suite.store.EXPECT().GetWalletByOrg(suite.ctx, "org_123").Return(&types.Wallet{
		Balance: 0.009,
	}, nil)

	resp, _, err := suite.controller.ChatCompletion(suite.ctx, suite.user, req, &ChatCompletionOptions{
		OrganizationID: "org_123",
	})
	suite.Error(err)
	suite.Nil(resp)
}

func (suite *ControllerSuite) Test_BasicInferenceWithKnowledge() {
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4TurboPreview,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: "Hello",
			},
		},
	}

	app := &types.App{
		ID:     "app_id",
		Global: true,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						ID: "0",
						Knowledge: []*types.AssistantKnowledge{
							{
								Name: "knowledge_name",
							},
						},
					},
				},
			},
		},
	}

	suite.store.EXPECT().GetAppWithTools(suite.ctx, "app_id").Return(app, nil)
	suite.store.EXPECT().ListSecrets(gomock.Any(), &store.ListSecretsQuery{
		Owner: suite.user.ID,
	}).Return([]*types.Secret{}, nil)

	plainTextKnowledge := "foo bar"

	knowledge := &types.Knowledge{
		ID:    "knowledge_id",
		AppID: "app_id",
		Source: types.KnowledgeSource{
			Text: &plainTextKnowledge,
		},
	}

	suite.store.EXPECT().LookupKnowledge(suite.ctx, &store.LookupKnowledgeQuery{
		Name:  "knowledge_name",
		AppID: "app_id",
	}).Return(knowledge, nil)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true)

	suite.openAiClient.EXPECT().CreateChatCompletion(suite.ctx, gomock.Any()).Return(openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: "Hello",
				},
			},
		},
	}, nil)

	resp, _, err := suite.controller.ChatCompletion(suite.ctx, suite.user, req, &ChatCompletionOptions{
		AppID:       "app_id",
		AssistantID: "0",
	})
	suite.NoError(err)
	suite.Equal(&openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: "Hello",
				},
			},
		},
	}, resp)
}

func (suite *ControllerSuite) Test_BasicInferenceWithKnowledge_MultiContent() {
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4TurboPreview,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleUser,
				MultiContent: []openai.ChatMessagePart{
					{
						Type: openai.ChatMessagePartTypeImageURL,
						ImageURL: &openai.ChatMessageImageURL{
							URL: "http://example.com/image.jpg",
						},
					},
					{
						Type: openai.ChatMessagePartTypeText,
						Text: "Initial user message",
					},
				},
			},
		},
	}

	app := &types.App{
		ID:     "app_id",
		Global: true,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						ID: "0",
						Knowledge: []*types.AssistantKnowledge{
							{
								Name: "knowledge_name",
							},
						},
					},
				},
			},
		},
	}

	suite.store.EXPECT().GetAppWithTools(suite.ctx, "app_id").Return(app, nil)
	suite.store.EXPECT().ListSecrets(gomock.Any(), &store.ListSecretsQuery{
		Owner: suite.user.ID,
	}).Return([]*types.Secret{}, nil)

	plainTextKnowledge := "plain text knowledge here"

	knowledge := &types.Knowledge{
		ID:    "knowledge_id",
		AppID: "app_id",
		Source: types.KnowledgeSource{
			Text: &plainTextKnowledge,
		},
	}

	suite.store.EXPECT().LookupKnowledge(suite.ctx, &store.LookupKnowledgeQuery{
		Name:  "knowledge_name",
		AppID: "app_id",
	}).Return(knowledge, nil)

	suite.openAiClient.EXPECT().BillingEnabled().Return(true)

	suite.openAiClient.EXPECT().CreateChatCompletion(suite.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			suite.Require().Equal(string(req.Messages[0].MultiContent[1].Type), "text")
			assert.Contains(suite.T(), req.Messages[0].MultiContent[1].Text, "plain text knowledge here")

			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							MultiContent: []openai.ChatMessagePart{
								{
									Type: openai.ChatMessagePartTypeText,
									Text: "Hello",
								},
							},
						},
					},
				},
			}, nil
		},
	)

	resp, _, err := suite.controller.ChatCompletion(suite.ctx, suite.user, req, &ChatCompletionOptions{
		AppID:       "app_id",
		AssistantID: "0",
	})
	suite.NoError(err)
	suite.Equal(&openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					MultiContent: []openai.ChatMessagePart{
						{
							Type: openai.ChatMessagePartTypeText,
							Text: "Hello",
						},
					},
				},
			},
		},
	}, resp)
}

func (suite *ControllerSuite) Test_EvaluateSecrets() {
	app := &types.App{
		ID:     "app_id",
		Global: true,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						ID: "0",
						Tools: []*types.Tool{
							{
								ID: "tool_id",
								Config: types.ToolConfig{
									API: &types.ToolAPIConfig{
										Model: "gpt-4o",
										Headers: map[string]string{
											"X-Secret-Key": "${API_KEY}",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	suite.store.EXPECT().ListSecrets(gomock.Any(), &store.ListSecretsQuery{
		Owner: suite.user.ID,
	}).Return([]*types.Secret{
		{
			Name:  "API_KEY",
			Value: []byte("secret_value"),
		},
	}, nil)

	app, err := suite.controller.evaluateSecrets(suite.ctx, suite.user, app)
	suite.NoError(err)

	// After evaluateSecrets, ${API_KEY} should be replaced with secret_value
	suite.Equal("secret_value", app.Config.Helix.Assistants[0].Tools[0].Config.API.Headers["X-Secret-Key"])
}

func Test_setSystemPrompt(t *testing.T) {
	type args struct {
		req          *openai.ChatCompletionRequest
		systemPrompt string
	}
	tests := []struct {
		name string
		args args
		want openai.ChatCompletionRequest
	}{
		{
			name: "No system prompt set and no messages",
			args: args{
				req:          &openai.ChatCompletionRequest{},
				systemPrompt: "",
			},
			want: openai.ChatCompletionRequest{},
		},
		{
			name: "System prompt set and message user only",
			args: args{
				req: &openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{
						{
							Role:    openai.ChatMessageRoleUser,
							Content: "Hello",
						},
					},
				},
				systemPrompt: "You are a helpful assistant.",
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{
						Role:    openai.ChatMessageRoleSystem,
						Content: "You are a helpful assistant.",
					},
					{
						Role:    openai.ChatMessageRoleUser,
						Content: "Hello",
					},
				},
			},
		},
		{
			name: "System prompt is set and request messages has system prompt",
			args: args{
				req: &openai.ChatCompletionRequest{
					Messages: []openai.ChatCompletionMessage{
						{
							Role:    openai.ChatMessageRoleSystem,
							Content: "Original system prompt",
						},
						{
							Role:    openai.ChatMessageRoleUser,
							Content: "Hello",
						},
					},
				},
				systemPrompt: "New system prompt",
			},
			want: openai.ChatCompletionRequest{
				Messages: []openai.ChatCompletionMessage{
					{
						Role:    openai.ChatMessageRoleSystem,
						Content: "New system prompt",
					},
					{
						Role:    openai.ChatMessageRoleUser,
						Content: "Hello",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := setSystemPrompt(tt.args.req, tt.args.systemPrompt); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("setSystemPrompt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_renderPrompt(t *testing.T) {
	prompt := "Hello, {{.LocalDate}} at {{.LocalTime}}"
	values := systemPromptValues{
		LocalDate: "2024-01-01",
		LocalTime: "12:00:00",
	}
	rendered, err := renderPrompt(prompt, values)
	assert.NoError(t, err)
	assert.Equal(t, "Hello, 2024-01-01 at 12:00:00", rendered)
}
