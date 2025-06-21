package agent

import (
	"context"
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
