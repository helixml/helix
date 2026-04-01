package evaluation

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
)

type RunnerSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockStore  *store.MockStore
	mockCtrl   *MockSessionController
	mockJudge  *mockJudge
	app        *types.App
	user       *types.User
}

func TestRunnerSuite(t *testing.T) { suite.Run(t, new(RunnerSuite)) }

func (s *RunnerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)
	s.mockCtrl = NewMockSessionController(s.ctrl)
	s.mockJudge = &mockJudge{}

	s.app = &types.App{
		ID:             "app-1",
		OrganizationID: "org-1",
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{
					SystemPrompt: "You are a test assistant",
				}},
			},
		},
	}
	s.user = &types.User{
		ID:   "user-1",
		Type: types.OwnerTypeUser,
	}
}

func (s *RunnerSuite) newRunnerConfig() *RunnerConfig {
	return &RunnerConfig{
		Store:      s.mockStore,
		Controller: s.mockCtrl,
		Judge:      s.mockJudge,
	}
}

// expectListLLMCalls sets up the mock to return LLM calls with the given token/cost values
func (s *RunnerSuite) expectListLLMCalls(promptTokens, completionTokens, totalTokens int64, totalCost float64) *gomock.Call {
	return s.mockStore.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).
		Return([]*types.LLMCall{{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
			TotalCost:        totalCost,
		}}, int64(1), nil)
}

// expectListLLMCallsEmpty sets up the mock to return no LLM calls
func (s *RunnerSuite) expectListLLMCallsEmpty() *gomock.Call {
	return s.mockStore.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).
		Return([]*types.LLMCall{}, int64(0), nil)
}

func (s *RunnerSuite) TestRunEvaluation_SingleQuestion_AllPass() {
	suite := &types.EvaluationSuite{
		Questions: []types.EvaluationQuestion{{
			ID:       "q1",
			Question: "What is 2+2?",
			Assertions: []types.EvaluationAssertion{
				{Type: types.EvaluationAssertionTypeContains, Value: "4"},
			},
		}},
	}

	run := &types.EvaluationRun{ID: "run-1", Status: types.EvaluationRunStatusPending}

	// Expect: update to running
	s.mockStore.EXPECT().UpdateEvaluationRun(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, r *types.EvaluationRun) (*types.EvaluationRun, error) {
			return r, nil
		}).Times(3) // running + progress + completed

	// Expect session creation
	s.mockCtrl.EXPECT().WriteSession(gomock.Any(), gomock.Any()).Return(nil)

	// Expect blocking session with response containing "4"
	s.mockCtrl.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).
		Return(&types.Interaction{
			ID:              "int-1",
			ResponseEntries: datatypes.JSON(`[{"type":"text","content":"The answer is 4"}]`),
		}, nil)

	s.expectListLLMCalls(100, 50, 150, 0.001)

	var progress []types.EvaluationRunProgress
	RunEvaluation(context.Background(), s.newRunnerConfig(), run, suite, s.app, s.user, func(p types.EvaluationRunProgress) {
		progress = append(progress, p)
	})

	assert.Equal(s.T(), types.EvaluationRunStatusCompleted, run.Status)
	assert.Equal(s.T(), 1, run.Summary.TotalQuestions)
	assert.Equal(s.T(), 1, run.Summary.Passed)
	assert.Equal(s.T(), 0, run.Summary.Failed)
	assert.Len(s.T(), run.Results, 1)
	assert.True(s.T(), run.Results[0].Passed)
	assert.Empty(s.T(), run.Error)

	// Should have running + per-question + completed progress
	assert.Len(s.T(), progress, 3)
	assert.Equal(s.T(), types.EvaluationRunStatusRunning, progress[0].Status)
	assert.Equal(s.T(), types.EvaluationRunStatusRunning, progress[1].Status)
	assert.Equal(s.T(), types.EvaluationRunStatusCompleted, progress[2].Status)
}

func (s *RunnerSuite) TestRunEvaluation_QuestionFails() {
	suite := &types.EvaluationSuite{
		Questions: []types.EvaluationQuestion{{
			ID:       "q1",
			Question: "What is 2+2?",
			Assertions: []types.EvaluationAssertion{
				{Type: types.EvaluationAssertionTypeContains, Value: "five"},
			},
		}},
	}

	run := &types.EvaluationRun{ID: "run-1"}

	s.mockStore.EXPECT().UpdateEvaluationRun(gomock.Any(), gomock.Any()).
		Return(&types.EvaluationRun{}, nil).Times(3)

	s.mockCtrl.EXPECT().WriteSession(gomock.Any(), gomock.Any()).Return(nil)
	s.mockCtrl.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).
		Return(&types.Interaction{
			ID:              "int-1",
			ResponseEntries: datatypes.JSON(`[{"type":"text","content":"The answer is 4"}]`),
		}, nil)
	s.expectListLLMCallsEmpty()

	RunEvaluation(context.Background(), s.newRunnerConfig(), run, suite, s.app, s.user, nil)

	assert.Equal(s.T(), types.EvaluationRunStatusCompleted, run.Status)
	assert.Equal(s.T(), 1, run.Summary.Failed)
	assert.Equal(s.T(), 0, run.Summary.Passed)
	assert.Contains(s.T(), run.Error, "1/1 questions failed")
}

func (s *RunnerSuite) TestRunEvaluation_MultipleQuestions() {
	suite := &types.EvaluationSuite{
		Questions: []types.EvaluationQuestion{
			{
				ID: "q1", Question: "Say hello",
				Assertions: []types.EvaluationAssertion{
					{Type: types.EvaluationAssertionTypeContains, Value: "hello"},
				},
			},
			{
				ID: "q2", Question: "Say goodbye",
				Assertions: []types.EvaluationAssertion{
					{Type: types.EvaluationAssertionTypeContains, Value: "goodbye"},
				},
			},
		},
	}

	run := &types.EvaluationRun{ID: "run-1"}

	s.mockStore.EXPECT().UpdateEvaluationRun(gomock.Any(), gomock.Any()).
		Return(&types.EvaluationRun{}, nil).AnyTimes()

	s.mockCtrl.EXPECT().WriteSession(gomock.Any(), gomock.Any()).Return(nil).Times(2)

	// First question passes, second fails
	gomock.InOrder(
		s.mockCtrl.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).
			Return(&types.Interaction{
				ID:              "int-1",
				ResponseEntries: datatypes.JSON(`[{"type":"text","content":"hello there!"}]`),
			}, nil),
		s.mockCtrl.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).
			Return(&types.Interaction{
				ID:              "int-2",
				ResponseEntries: datatypes.JSON(`[{"type":"text","content":"see you later"}]`),
			}, nil),
	)
	s.mockStore.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).
		Return([]*types.LLMCall{}, int64(0), nil).Times(2)

	RunEvaluation(context.Background(), s.newRunnerConfig(), run, suite, s.app, s.user, nil)

	assert.Equal(s.T(), types.EvaluationRunStatusCompleted, run.Status)
	assert.Equal(s.T(), 2, run.Summary.TotalQuestions)
	assert.Equal(s.T(), 1, run.Summary.Passed)
	assert.Equal(s.T(), 1, run.Summary.Failed)
	assert.Len(s.T(), run.Results, 2)
	assert.True(s.T(), run.Results[0].Passed)
	assert.False(s.T(), run.Results[1].Passed)
}

func (s *RunnerSuite) TestRunEvaluation_Cancelled() {
	suite := &types.EvaluationSuite{
		Questions: []types.EvaluationQuestion{
			{ID: "q1", Question: "test"},
		},
	}

	run := &types.EvaluationRun{ID: "run-1"}

	// running update + cancelled update
	s.mockStore.EXPECT().UpdateEvaluationRun(gomock.Any(), gomock.Any()).
		Return(&types.EvaluationRun{}, nil).Times(2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	RunEvaluation(ctx, s.newRunnerConfig(), run, suite, s.app, s.user, nil)

	assert.Equal(s.T(), types.EvaluationRunStatusCancelled, run.Status)
	assert.Equal(s.T(), "cancelled", run.Error)
}

func (s *RunnerSuite) TestRunEvaluation_SessionError() {
	suite := &types.EvaluationSuite{
		Questions: []types.EvaluationQuestion{{
			ID:       "q1",
			Question: "test",
			Assertions: []types.EvaluationAssertion{
				{Type: types.EvaluationAssertionTypeContains, Value: "something"},
			},
		}},
	}

	run := &types.EvaluationRun{ID: "run-1"}

	s.mockStore.EXPECT().UpdateEvaluationRun(gomock.Any(), gomock.Any()).
		Return(&types.EvaluationRun{}, nil).AnyTimes()

	s.mockCtrl.EXPECT().WriteSession(gomock.Any(), gomock.Any()).Return(nil)
	s.mockCtrl.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).
		Return(nil, assert.AnError)

	RunEvaluation(context.Background(), s.newRunnerConfig(), run, suite, s.app, s.user, nil)

	assert.Equal(s.T(), types.EvaluationRunStatusCompleted, run.Status)
	assert.Len(s.T(), run.Results, 1)
	assert.False(s.T(), run.Results[0].Passed)
	assert.NotEmpty(s.T(), run.Results[0].Error)
}

func (s *RunnerSuite) TestRunEvaluation_WriteSessionError() {
	suite := &types.EvaluationSuite{
		Questions: []types.EvaluationQuestion{{
			ID: "q1", Question: "test",
		}},
	}

	run := &types.EvaluationRun{ID: "run-1"}

	s.mockStore.EXPECT().UpdateEvaluationRun(gomock.Any(), gomock.Any()).
		Return(&types.EvaluationRun{}, nil).AnyTimes()

	s.mockCtrl.EXPECT().WriteSession(gomock.Any(), gomock.Any()).
		Return(assert.AnError)

	RunEvaluation(context.Background(), s.newRunnerConfig(), run, suite, s.app, s.user, nil)

	assert.Equal(s.T(), types.EvaluationRunStatusCompleted, run.Status)
	assert.Len(s.T(), run.Results, 1)
	assert.Contains(s.T(), run.Results[0].Error, "failed to create session")
}

func (s *RunnerSuite) TestRunEvaluation_NoProgressCallback() {
	suite := &types.EvaluationSuite{
		Questions: []types.EvaluationQuestion{{
			ID: "q1", Question: "test",
		}},
	}

	run := &types.EvaluationRun{ID: "run-1"}

	s.mockStore.EXPECT().UpdateEvaluationRun(gomock.Any(), gomock.Any()).
		Return(&types.EvaluationRun{}, nil).AnyTimes()

	s.mockCtrl.EXPECT().WriteSession(gomock.Any(), gomock.Any()).Return(nil)
	s.mockCtrl.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).
		Return(&types.Interaction{
			ResponseEntries: datatypes.JSON(`[{"type":"text","content":"response"}]`),
		}, nil)
	s.expectListLLMCallsEmpty()

	// Should not panic with nil callback
	RunEvaluation(context.Background(), s.newRunnerConfig(), run, suite, s.app, s.user, nil)

	assert.Equal(s.T(), types.EvaluationRunStatusCompleted, run.Status)
}

func (s *RunnerSuite) TestRunEvaluation_WithLLMJudgeAssertion() {
	s.mockJudge.passed = true
	s.mockJudge.reasoning = "PASS: response is good"

	suite := &types.EvaluationSuite{
		Questions: []types.EvaluationQuestion{{
			ID:       "q1",
			Question: "Be helpful",
			Assertions: []types.EvaluationAssertion{
				{Type: types.EvaluationAssertionTypeLLMJudge, Value: "response is helpful"},
			},
		}},
	}

	run := &types.EvaluationRun{ID: "run-1"}

	s.mockStore.EXPECT().UpdateEvaluationRun(gomock.Any(), gomock.Any()).
		Return(&types.EvaluationRun{}, nil).AnyTimes()

	s.mockCtrl.EXPECT().WriteSession(gomock.Any(), gomock.Any()).Return(nil)
	s.mockCtrl.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).
		Return(&types.Interaction{
			ResponseEntries: datatypes.JSON(`[{"type":"text","content":"I'm happy to help!"}]`),
		}, nil)
	s.expectListLLMCallsEmpty()

	RunEvaluation(context.Background(), s.newRunnerConfig(), run, suite, s.app, s.user, nil)

	assert.Equal(s.T(), 1, run.Summary.Passed)
	assert.True(s.T(), run.Results[0].Passed)
	assert.Len(s.T(), run.Results[0].AssertionResults, 1)
	assert.True(s.T(), run.Results[0].AssertionResults[0].Passed)
}

func (s *RunnerSuite) TestRunEvaluation_SkillsTracked() {
	suite := &types.EvaluationSuite{
		Questions: []types.EvaluationQuestion{{
			ID:       "q1",
			Question: "Search for something",
			Assertions: []types.EvaluationAssertion{
				{Type: types.EvaluationAssertionTypeSkillUsed, Value: "web_search"},
			},
		}},
	}

	run := &types.EvaluationRun{ID: "run-1"}

	s.mockStore.EXPECT().UpdateEvaluationRun(gomock.Any(), gomock.Any()).
		Return(&types.EvaluationRun{}, nil).AnyTimes()

	s.mockCtrl.EXPECT().WriteSession(gomock.Any(), gomock.Any()).Return(nil)
	s.mockCtrl.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).
		Return(&types.Interaction{
			ResponseEntries: datatypes.JSON(`[{"type":"text","content":"search results"}]`),
			ToolCalls: []openai.ToolCall{
				{Function: openai.FunctionCall{Name: "web_search"}},
			},
		}, nil)
	s.expectListLLMCallsEmpty()

	RunEvaluation(context.Background(), s.newRunnerConfig(), run, suite, s.app, s.user, nil)

	assert.True(s.T(), run.Results[0].Passed)
	assert.Equal(s.T(), []string{"web_search"}, run.Results[0].SkillsUsed)
	assert.Equal(s.T(), []string{"web_search"}, run.Summary.SkillsUsed)
}

func (s *RunnerSuite) TestRunEvaluation_TokensAndCostAggregated() {
	suite := &types.EvaluationSuite{
		Questions: []types.EvaluationQuestion{
			{ID: "q1", Question: "test1"},
			{ID: "q2", Question: "test2"},
		},
	}

	run := &types.EvaluationRun{ID: "run-1"}

	s.mockStore.EXPECT().UpdateEvaluationRun(gomock.Any(), gomock.Any()).
		Return(&types.EvaluationRun{}, nil).AnyTimes()

	s.mockCtrl.EXPECT().WriteSession(gomock.Any(), gomock.Any()).Return(nil).Times(2)

	gomock.InOrder(
		s.mockCtrl.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).
			Return(&types.Interaction{
				ID:              "int-1",
				ResponseEntries: datatypes.JSON(`[{"type":"text","content":"r1"}]`),
			}, nil),
		s.mockCtrl.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).
			Return(&types.Interaction{
				ID:              "int-2",
				ResponseEntries: datatypes.JSON(`[{"type":"text","content":"r2"}]`),
			}, nil),
	)

	// LLM call records for each interaction (source of truth for usage)
	gomock.InOrder(
		s.mockStore.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).
			Return([]*types.LLMCall{{
				PromptTokens:     60,
				CompletionTokens: 40,
				TotalTokens:      100,
				TotalCost:        0.001,
			}}, int64(1), nil),
		s.mockStore.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).
			Return([]*types.LLMCall{{
				PromptTokens:     120,
				CompletionTokens: 80,
				TotalTokens:      200,
				TotalCost:        0.002,
			}}, int64(1), nil),
	)

	RunEvaluation(context.Background(), s.newRunnerConfig(), run, suite, s.app, s.user, nil)

	assert.Equal(s.T(), 300, run.Summary.TotalTokens)
	assert.InDelta(s.T(), 0.003, run.Summary.TotalCost, 0.0001)
	assert.GreaterOrEqual(s.T(), run.Summary.TotalDurationMs, int64(0))
}
