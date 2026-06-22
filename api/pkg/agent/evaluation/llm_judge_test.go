package evaluation

import (
	"context"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type LLMJudgeSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	completer *MockChatCompleter
}

func TestLLMJudgeSuite(t *testing.T) { suite.Run(t, new(LLMJudgeSuite)) }

func (s *LLMJudgeSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.completer = NewMockChatCompleter(s.ctrl)
}

// --- pickJudgeModel ---

func (s *LLMJudgeSuite) TestPickJudgeModel_NoAssistants() {
	app := &types.App{}
	model, provider := pickJudgeModel(app)
	assert.Empty(s.T(), model)
	assert.Empty(s.T(), provider)
}

func (s *LLMJudgeSuite) TestPickJudgeModel_SmallGenerationModel() {
	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{
					SmallGenerationModel:         "small-gen",
					SmallGenerationModelProvider: "provider-a",
					GenerationModel:              "gen-model",
					Model:                        "main-model",
				}},
			},
		},
	}
	model, provider := pickJudgeModel(app)
	assert.Equal(s.T(), "small-gen", model)
	assert.Equal(s.T(), "provider-a", provider)
}

func (s *LLMJudgeSuite) TestPickJudgeModel_FallsBackToGenerationModel() {
	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{
					GenerationModel:         "gen-model",
					GenerationModelProvider: "provider-b",
					Model:                   "main-model",
				}},
			},
		},
	}
	model, provider := pickJudgeModel(app)
	assert.Equal(s.T(), "gen-model", model)
	assert.Equal(s.T(), "provider-b", provider)
}

func (s *LLMJudgeSuite) TestPickJudgeModel_FallsBackToModel() {
	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{
					Model:    "main-model",
					Provider: "provider-c",
				}},
			},
		},
	}
	model, provider := pickJudgeModel(app)
	assert.Equal(s.T(), "main-model", model)
	assert.Equal(s.T(), "provider-c", provider)
}

func (s *LLMJudgeSuite) TestPickJudgeModel_NoModelsConfigured() {
	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{}},
			},
		},
	}
	model, provider := pickJudgeModel(app)
	assert.Empty(s.T(), model)
	assert.Empty(s.T(), provider)
}

// --- NewControllerLLMJudge ---

func (s *LLMJudgeSuite) TestNewControllerLLMJudge_ReturnsNilWhenNoModel() {
	app := &types.App{}
	judge := NewControllerLLMJudge(s.completer, &types.User{}, app)
	assert.Nil(s.T(), judge)
}

func (s *LLMJudgeSuite) TestNewControllerLLMJudge_ReturnsJudgeWithModel() {
	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{
					Model: "gpt-4",
				}},
			},
		},
	}
	judge := NewControllerLLMJudge(s.completer, &types.User{}, app)
	assert.NotNil(s.T(), judge)
}

// --- Judge method ---

func (s *LLMJudgeSuite) TestJudge_Pass() {
	user := &types.User{ID: "user-1"}
	app := &types.App{
		OrganizationID: "org-1",
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{
					Model:    "test-model",
					Provider: "test-provider",
				}},
			},
		},
	}

	judge := NewControllerLLMJudge(s.completer, user, app)

	s.completer.EXPECT().ChatCompletion(
		gomock.Any(), gomock.Eq(user), gomock.Any(), gomock.Any(),
	).Return(&openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{
			Message: openai.ChatCompletionMessage{
				Content: "PASS: The response meets the criteria.",
			},
		}},
	}, &openai.ChatCompletionRequest{}, nil)

	passed, reasoning, err := judge.Judge(context.Background(), "", "response text", "be helpful")

	assert.NoError(s.T(), err)
	assert.True(s.T(), passed)
	assert.Contains(s.T(), reasoning, "PASS")
}

func (s *LLMJudgeSuite) TestJudge_Fail() {
	user := &types.User{ID: "user-1"}
	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{Model: "test-model"}},
			},
		},
	}

	judge := NewControllerLLMJudge(s.completer, user, app)

	s.completer.EXPECT().ChatCompletion(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(&openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{
			Message: openai.ChatCompletionMessage{
				Content: "FAIL: The response is off-topic.",
			},
		}},
	}, &openai.ChatCompletionRequest{}, nil)

	passed, reasoning, err := judge.Judge(context.Background(), "", "bad response", "be on topic")

	assert.NoError(s.T(), err)
	assert.False(s.T(), passed)
	assert.Contains(s.T(), reasoning, "FAIL")
}

func (s *LLMJudgeSuite) TestJudge_Error() {
	user := &types.User{ID: "user-1"}
	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{Model: "test-model"}},
			},
		},
	}

	judge := NewControllerLLMJudge(s.completer, user, app)

	s.completer.EXPECT().ChatCompletion(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(nil, nil, fmt.Errorf("connection refused"))

	passed, _, err := judge.Judge(context.Background(), "", "response", "criteria")

	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "LLM judge call failed")
	assert.False(s.T(), passed)
}

func (s *LLMJudgeSuite) TestJudge_NoChoices() {
	user := &types.User{ID: "user-1"}
	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{Model: "test-model"}},
			},
		},
	}

	judge := NewControllerLLMJudge(s.completer, user, app)

	s.completer.EXPECT().ChatCompletion(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(&openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{},
	}, &openai.ChatCompletionRequest{}, nil)

	passed, _, err := judge.Judge(context.Background(), "", "response", "criteria")

	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "no choices")
	assert.False(s.T(), passed)
}

func (s *LLMJudgeSuite) TestJudge_PassWithLeadingWhitespace() {
	user := &types.User{ID: "user-1"}
	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{Model: "test-model"}},
			},
		},
	}

	judge := NewControllerLLMJudge(s.completer, user, app)

	s.completer.EXPECT().ChatCompletion(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(&openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{
			Message: openai.ChatCompletionMessage{
				Content: "  \n  Pass - the response is valid",
			},
		}},
	}, &openai.ChatCompletionRequest{}, nil)

	passed, _, err := judge.Judge(context.Background(), "", "response", "criteria")

	assert.NoError(s.T(), err)
	assert.True(s.T(), passed)
}

func (s *LLMJudgeSuite) TestJudge_UsesCorrectModelAndProvider() {
	user := &types.User{ID: "user-1"}
	app := &types.App{
		OrganizationID: "org-123",
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{
					SmallGenerationModel:         "small-model",
					SmallGenerationModelProvider: "my-provider",
				}},
			},
		},
	}

	judge := NewControllerLLMJudge(s.completer, user, app)

	s.completer.EXPECT().ChatCompletion(
		gomock.Any(), gomock.Eq(user),
		gomock.AssignableToTypeOf(openai.ChatCompletionRequest{}),
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ *types.User, req openai.ChatCompletionRequest, opts *controller.ChatCompletionOptions) (*openai.ChatCompletionResponse, *openai.ChatCompletionRequest, error) {
		assert.Equal(s.T(), "small-model", req.Model)
		assert.Equal(s.T(), "my-provider", opts.Provider)
		assert.Equal(s.T(), "org-123", opts.OrganizationID)

		return &openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{{
				Message: openai.ChatCompletionMessage{Content: "PASS"},
			}},
		}, &req, nil
	})

	passed, _, err := judge.Judge(context.Background(), "", "response", "criteria")
	assert.NoError(s.T(), err)
	assert.True(s.T(), passed)
}
