package agent

import (
	"context"
	"testing"

	helix_openai "github.com/helixml/helix/api/pkg/openai"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

func TestSkillDirectRunner(t *testing.T) {
	suite.Run(t, new(SkillDirectRunnerTestSuite))
}

type SkillDirectRunnerTestSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	openaiClient *helix_openai.MockClient
	llm          *LLM
	agent        *Agent
	emitter      *MockStepInfoEmitter
}

func (s *SkillDirectRunnerTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.openaiClient = helix_openai.NewMockClient(s.ctrl)
	s.llm = NewLLM(
		&LLMModelConfig{
			Client: s.openaiClient,
			Model:  "gpt-4o-mini",
		},
		&LLMModelConfig{
			Client: s.openaiClient,
			Model:  "gpt-4o-mini",
		},
		&LLMModelConfig{
			Client: s.openaiClient,
			Model:  "gpt-4o-mini",
		},
		&LLMModelConfig{
			Client: s.openaiClient,
			Model:  "gpt-4o-mini",
		},
	)
	s.emitter = NewMockStepInfoEmitter(s.ctrl)

	s.agent = NewAgent(s.emitter, "", []Skill{}, 10)

}

func (s *SkillDirectRunnerTestSuite) Test_SkillDirectRunner_NoTools() {
	skills := []Skill{
		{
			Name:  "TestSkill",
			Tools: []Tool{},
		},
	}

	toolCall := openai.ToolCall{
		ID: "test-tool-call",
		Function: openai.FunctionCall{
			Name:      "test-tool",
			Arguments: "{}",
		},
	}

	s.agent.skills = skills

	result, err := s.agent.SkillDirectRunner(context.Background(), Meta{}, &skills[0], toolCall)

	s.Require().Error(err)
	s.Require().Equal(err.Error(), "skill TestSkill has no tools")
	s.Require().Nil(result)
}

func (s *SkillDirectRunnerTestSuite) Test_SkillDirectRunner_MultipleTools() {
	mockTool := NewMockTool(s.ctrl)

	skill := Skill{
		Name: "TestSkill",
		Tools: []Tool{
			mockTool,
			mockTool,
		},
	}

	toolCall := openai.ToolCall{
		ID: "test-tool-call",
		Function: openai.FunctionCall{
			Name:      "test-tool",
			Arguments: "{}",
		},
	}

	result, err := s.agent.SkillDirectRunner(context.Background(), Meta{}, &skill, toolCall)

	s.Require().Error(err)
	s.Require().Equal(err.Error(), "skill TestSkill has more than one tool, direct skills are not supported")
	s.Require().Nil(result)
}

func (s *SkillDirectRunnerTestSuite) Test_SkillDirectRunner_ExecuteSuccess() {
	mockTool := NewMockTool(s.ctrl)

	mockTool.EXPECT().Name().Return("TestTool")
	mockTool.EXPECT().Icon().Return("🧮")
	mockTool.EXPECT().Execute(gomock.Any(), gomock.Any(), gomock.Any()).Return("test-output", nil)

	s.emitter.EXPECT().EmitStepInfo(gomock.Any(), gomock.Any()).Return(nil)

	skill := Skill{
		Name: "TestSkill",
		Tools: []Tool{
			mockTool,
		},
	}

	toolCall := openai.ToolCall{
		ID: "test-tool-call",
		Function: openai.FunctionCall{
			Name:      "test-tool",
			Arguments: `{"expression": "2 + 2"}`,
		},
	}

	result, err := s.agent.SkillDirectRunner(context.Background(), Meta{}, &skill, toolCall)

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(result.Role, openai.ChatMessageRoleTool)
	s.Require().Equal(result.Content, "test-output")
	s.Require().Equal(result.ToolCallID, toolCall.ID)
}
