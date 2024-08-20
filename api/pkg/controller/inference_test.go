package controller

import (
	"context"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	openai "github.com/lukemarsden/go-openai2"
	"github.com/stretchr/testify/suite"
)

func TestControllerSuite(t *testing.T) {
	suite.Run(t, new(ControllerSuite))
}

type ControllerSuite struct {
	suite.Suite

	ctx context.Context

	store        *store.MockStore
	pubsub       pubsub.PubSub
	openAiClient *oai.MockClient
	rag          *rag.MockRAG
	user         *types.User

	controller *Controller
}

func (suite *ControllerSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())

	suite.ctx = context.Background()
	suite.store = store.NewMockStore(ctrl)
	ps, err := pubsub.New(suite.T().TempDir())
	suite.NoError(err)

	suite.openAiClient = oai.NewMockClient(ctrl)
	suite.pubsub = ps

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
	cfg.Inference.Provider = config.ProviderTogetherAI

	c, err := NewController(context.Background(), ControllerOptions{
		Config:       cfg,
		Store:        suite.store,
		Janitor:      janitor.NewJanitor(config.Janitor{}),
		OpenAIClient: suite.openAiClient,
		Filestore:    filestoreMock,
		Extractor:    extractorMock,
		RAG:          suite.rag,
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

	suite.openAiClient.EXPECT().CreateChatCompletion(suite.ctx, gomock.Any()).Return(openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: "Hello",
				},
			},
		},
	}, nil)

	resp, err := suite.controller.ChatCompletion(suite.ctx, suite.user, req, &ChatCompletionOptions{})
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

	suite.store.EXPECT().GetApp(suite.ctx, "app_id").Return(app, nil)

	plainTextKnowledge := "foo bar"

	knowledge := &types.Knowledge{
		ID:    "knowledge_id",
		AppID: "app_id",
		Source: types.KnowledgeSource{
			Content: &plainTextKnowledge,
		},
	}

	suite.store.EXPECT().LookupKnowledge(suite.ctx, &store.LookupKnowledgeQuery{
		Name:  "knowledge_name",
		AppID: "app_id",
	}).Return(knowledge, nil)

	suite.openAiClient.EXPECT().CreateChatCompletion(suite.ctx, gomock.Any()).Return(openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: "Hello",
				},
			},
		},
	}, nil)

	resp, err := suite.controller.ChatCompletion(suite.ctx, suite.user, req, &ChatCompletionOptions{
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
