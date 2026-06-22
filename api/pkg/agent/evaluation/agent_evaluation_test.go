package evaluation

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type AssertionSuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	judge *mockJudge
}

func TestAssertionSuite(t *testing.T) { suite.Run(t, new(AssertionSuite)) }

func (s *AssertionSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.judge = &mockJudge{}
}

// mockJudge is a simple test double for LLMJudge
type mockJudge struct {
	passed    bool
	reasoning string
	err       error
}

func (m *mockJudge) Judge(_ context.Context, _, _, _ string) (bool, string, error) {
	return m.passed, m.reasoning, m.err
}

// --- Contains ---

func (s *AssertionSuite) TestContains_Pass() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeContains,
		Value: "hello",
	}, "Hello world", nil, nil)

	assert.True(s.T(), result.Passed)
	assert.Empty(s.T(), result.Details)
	assert.Equal(s.T(), types.EvaluationAssertionTypeContains, result.AssertionType)
}

func (s *AssertionSuite) TestContains_Fail() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeContains,
		Value: "goodbye",
	}, "Hello world", nil, nil)

	assert.False(s.T(), result.Passed)
	assert.Contains(s.T(), result.Details, "does not contain")
}

func (s *AssertionSuite) TestContains_CaseInsensitive() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeContains,
		Value: "HELLO",
	}, "hello world", nil, nil)

	assert.True(s.T(), result.Passed)
}

// --- NotContains ---

func (s *AssertionSuite) TestNotContains_Pass() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeNotContains,
		Value: "goodbye",
	}, "Hello world", nil, nil)

	assert.True(s.T(), result.Passed)
}

func (s *AssertionSuite) TestNotContains_Fail() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeNotContains,
		Value: "hello",
	}, "Hello world", nil, nil)

	assert.False(s.T(), result.Passed)
	assert.Contains(s.T(), result.Details, "should not")
}

// --- Regex ---

func (s *AssertionSuite) TestRegex_Pass() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeRegex,
		Value: `\d{3}-\d{4}`,
	}, "Call 555-1234 now", nil, nil)

	assert.True(s.T(), result.Passed)
}

func (s *AssertionSuite) TestRegex_Fail() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeRegex,
		Value: `^\d+$`,
	}, "not a number", nil, nil)

	assert.False(s.T(), result.Passed)
	assert.Contains(s.T(), result.Details, "does not match regex")
}

func (s *AssertionSuite) TestRegex_InvalidPattern() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeRegex,
		Value: `[invalid`,
	}, "anything", nil, nil)

	assert.False(s.T(), result.Passed)
	assert.Contains(s.T(), result.Details, "invalid regex")
}

// --- LLM Judge ---

func (s *AssertionSuite) TestLLMJudge_Pass() {
	s.judge.passed = true
	s.judge.reasoning = "PASS: looks good"

	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeLLMJudge,
		Value: "response should be polite",
	}, "Thank you for your help!", nil, s.judge)

	assert.True(s.T(), result.Passed)
	assert.Equal(s.T(), "PASS: looks good", result.Details)
}

func (s *AssertionSuite) TestLLMJudge_Fail() {
	s.judge.passed = false
	s.judge.reasoning = "FAIL: response was rude"

	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeLLMJudge,
		Value: "response should be polite",
	}, "Go away!", nil, s.judge)

	assert.False(s.T(), result.Passed)
}

func (s *AssertionSuite) TestLLMJudge_NilJudge() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeLLMJudge,
		Value: "criteria",
	}, "response", nil, nil)

	assert.False(s.T(), result.Passed)
	assert.Equal(s.T(), "LLM judge not available", result.Details)
}

func (s *AssertionSuite) TestLLMJudge_Error() {
	s.judge.err = assert.AnError

	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeLLMJudge,
		Value: "criteria",
	}, "response", nil, s.judge)

	assert.False(s.T(), result.Passed)
	assert.Contains(s.T(), result.Details, "LLM judge error")
}

func (s *AssertionSuite) TestLLMJudge_CustomPrompt() {
	// The custom prompt is used via LLMJudgePrompt field
	// We verify the assertion value is still set correctly in the result
	s.judge.passed = true
	s.judge.reasoning = "custom prompt worked"

	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:           types.EvaluationAssertionTypeLLMJudge,
		Value:          "original criteria",
		LLMJudgePrompt: "custom detailed prompt",
	}, "response", nil, s.judge)

	assert.True(s.T(), result.Passed)
	assert.Equal(s.T(), "original criteria", result.AssertionValue)
}

// --- SkillUsed ---

func (s *AssertionSuite) TestSkillUsed_Pass() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeSkillUsed,
		Value: "web_search",
	}, "", []string{"web_search", "calculator"}, nil)

	assert.True(s.T(), result.Passed)
}

func (s *AssertionSuite) TestSkillUsed_CaseInsensitive() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeSkillUsed,
		Value: "Web_Search",
	}, "", []string{"web_search"}, nil)

	assert.True(s.T(), result.Passed)
}

func (s *AssertionSuite) TestSkillUsed_Fail() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeSkillUsed,
		Value: "web_search",
	}, "", []string{"calculator"}, nil)

	assert.False(s.T(), result.Passed)
	assert.Contains(s.T(), result.Details, "was not used")
}

func (s *AssertionSuite) TestSkillUsed_EmptySkills() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  types.EvaluationAssertionTypeSkillUsed,
		Value: "web_search",
	}, "", nil, nil)

	assert.False(s.T(), result.Passed)
}

// --- Unknown type ---

func (s *AssertionSuite) TestUnknownType() {
	result := CheckAssertion(context.Background(), types.EvaluationAssertion{
		Type:  "bogus_type",
		Value: "foo",
	}, "response", nil, nil)

	assert.False(s.T(), result.Passed)
	assert.Contains(s.T(), result.Details, "unknown assertion type")
}

// --- CheckAllAssertions ---

func (s *AssertionSuite) TestCheckAllAssertions_AllPass() {
	assertions := []types.EvaluationAssertion{
		{Type: types.EvaluationAssertionTypeContains, Value: "hello"},
		{Type: types.EvaluationAssertionTypeNotContains, Value: "goodbye"},
	}

	results, allPassed := CheckAllAssertions(context.Background(), assertions, "hello world", nil, nil)

	assert.True(s.T(), allPassed)
	assert.Len(s.T(), results, 2)
	assert.True(s.T(), results[0].Passed)
	assert.True(s.T(), results[1].Passed)
}

func (s *AssertionSuite) TestCheckAllAssertions_OneFails() {
	assertions := []types.EvaluationAssertion{
		{Type: types.EvaluationAssertionTypeContains, Value: "hello"},
		{Type: types.EvaluationAssertionTypeContains, Value: "missing"},
	}

	results, allPassed := CheckAllAssertions(context.Background(), assertions, "hello world", nil, nil)

	assert.False(s.T(), allPassed)
	assert.Len(s.T(), results, 2)
	assert.True(s.T(), results[0].Passed)
	assert.False(s.T(), results[1].Passed)
}

func (s *AssertionSuite) TestCheckAllAssertions_Empty() {
	results, allPassed := CheckAllAssertions(context.Background(), nil, "response", nil, nil)

	assert.True(s.T(), allPassed)
	assert.Nil(s.T(), results)
}

// --- Helper functions ---

func (s *AssertionSuite) TestExtractSkillNames() {
	interaction := &types.Interaction{
		ToolCalls: []openai.ToolCall{
			{Function: openai.FunctionCall{Name: "search"}},
			{Function: openai.FunctionCall{Name: "calculator"}},
			{Function: openai.FunctionCall{Name: "search"}}, // duplicate
			{Function: openai.FunctionCall{Name: ""}},       // empty
		},
	}

	names := extractSkillNames(interaction)
	assert.Equal(s.T(), []string{"search", "calculator"}, names)
}

func (s *AssertionSuite) TestExtractSkillNames_NoToolCalls() {
	interaction := &types.Interaction{}
	assert.Nil(s.T(), extractSkillNames(interaction))
}

func (s *AssertionSuite) TestEstimateCost() {
	usage := types.Usage{
		PromptTokens:     1_000_000,
		CompletionTokens: 1_000_000,
	}
	cost := estimateCost(usage)
	// $3/1M input + $15/1M output = $18
	assert.InDelta(s.T(), 18.0, cost, 0.001)
}

func (s *AssertionSuite) TestEstimateCost_Zero() {
	assert.Equal(s.T(), 0.0, estimateCost(types.Usage{}))
}

func (s *AssertionSuite) TestTruncate() {
	assert.Equal(s.T(), "short", truncate("short", 10))
	assert.Equal(s.T(), "1234567...", truncate("1234567890123", 10))
	assert.Equal(s.T(), "", truncate("", 10))
}
