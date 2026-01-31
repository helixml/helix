package agent

import (
	"context"
	"strings"
	"testing"

	helix_openai "github.com/helixml/helix/api/pkg/openai"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

func TestLLMClient(t *testing.T) {
	suite.Run(t, new(LLMClientTestSuite))
}

type LLMClientTestSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	openaiClient *helix_openai.MockClient
	llm          *LLM
}

func (s *LLMClientTestSuite) SetupTest() {
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
}

func (s *LLMClientTestSuite) Test_LLMClient_New() {
	s.openaiClient.EXPECT().CreateChatCompletion(context.Background(), gomock.Any()).Return(openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: "Hello, world!",
				},
			},
		},
	}, nil)

	llm, err := s.llm.New(context.Background(), s.llm.GenerationModel, openai.ChatCompletionRequest{
		Model: "gpt-4o-mini",
	})

	s.Require().NoError(err)
	s.Require().NotNil(llm)
}

func (s *LLMClientTestSuite) Test_LLMClient_NewToolCall() {
	s.openaiClient.EXPECT().CreateChatCompletion(context.Background(), gomock.Any()).Return(openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Role: openai.ChatMessageRoleAssistant,
					ToolCalls: []openai.ToolCall{
						{
							ID: "tool_call_id",
							Function: openai.FunctionCall{
								Name: "func-1",
							},
						},
					},
				},
			},
		},
	}, nil)

	llm, err := s.llm.New(context.Background(), s.llm.GenerationModel, openai.ChatCompletionRequest{
		Model: "gpt-4o-mini",
	})

	s.Require().NoError(err)
	s.Require().NotNil(llm)

	// Check that the tool call ID is set
	s.Require().Equal("tool_call_id", llm.Choices[0].Message.ToolCalls[0].ID)
	// Check function name
	s.Require().Equal("func-1", llm.Choices[0].Message.ToolCalls[0].Function.Name)
}

func (s *LLMClientTestSuite) Test_LLMClient_NewToolCall_NoID() {
	s.openaiClient.EXPECT().CreateChatCompletion(context.Background(), gomock.Any()).Return(openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Role: openai.ChatMessageRoleAssistant,
					ToolCalls: []openai.ToolCall{
						{
							ID: "", // No ID
							Function: openai.FunctionCall{
								Name: "func-1",
							},
						},
					},
				},
			},
		},
	}, nil)

	llm, err := s.llm.New(context.Background(), s.llm.GenerationModel, openai.ChatCompletionRequest{
		Model: "gpt-4o-mini",
	})

	s.Require().NoError(err)
	s.Require().NotNil(llm)

	// Check that the tool call ID is set
	s.Require().NotEmpty(llm.Choices[0].Message.ToolCalls[0].ID)
	// It needs to start with call_
	s.Require().True(strings.HasPrefix(llm.Choices[0].Message.ToolCalls[0].ID, "call_"))
	// Check function name
	s.Require().Equal("func-1", llm.Choices[0].Message.ToolCalls[0].Function.Name)
}
