package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	helix_openai "github.com/helixml/helix/api/pkg/openai"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

func TestAgent(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}

type AgentTestSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	openaiClient *helix_openai.MockClient
	llm          *LLM
}

func (s *AgentTestSuite) SetupTest() {
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

func (s *AgentTestSuite) Test_Agent_NoSkills() {
	agent := NewAgent(NewLogStepInfoEmitter(), "Test prompt", []Skill{}, 10)

	respCh := make(chan Response)

	// Should be direct call to LLM
	s.openaiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).Return(openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: "Test response",
				},
			},
		},
	}, nil)

	go func() {
		defer close(respCh)

		agent.Run(context.Background(), Meta{}, s.llm, &MessageList{
			Messages: []*openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "Test question",
				},
			},
		}, &MemoryBlock{}, respCh)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	select {
	case <-ctx.Done():
		s.Require().Fail("Context done")
	case resp := <-respCh:
		s.Require().Equal(resp.Content, "Test response")
		s.Require().Equal(resp.Type, ResponseTypePartialText)
	}
}

func TestSkillValidation(t *testing.T) {
	// Test case 1: Missing Description
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected panic due to missing Description, but no panic occurred")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("Unexpected panic type: %T", r)
		}
		if !strings.Contains(msg, "missing a Description") {
			t.Fatalf("Unexpected panic message: %s", msg)
		}
	}()

	skill := Skill{
		Name: "TestSkill",
		// Description intentionally missing
		SystemPrompt: "Test system prompt",
	}

	stepInfoEmitter := NewLogStepInfoEmitter()

	_ = NewAgent(stepInfoEmitter, "Test prompt", []Skill{skill}, 10)

	// This line should not be reached due to the panic
	t.Fatal("Test should have panicked before reaching this point")
}

func TestSkillValidationSystemPrompt(t *testing.T) {
	// Test case 2: Missing SystemPrompt
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Expected panic due to missing SystemPrompt, but no panic occurred")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("Unexpected panic type: %T", r)
		}
		if !strings.Contains(msg, "missing a SystemPrompt") {
			t.Fatalf("Unexpected panic message: %s", msg)
		}
	}()

	skill := Skill{
		Name:        "TestSkill",
		Description: "Test description",
		// SystemPrompt intentionally missing
	}
	stepInfoEmitter := NewLogStepInfoEmitter()

	_ = NewAgent(stepInfoEmitter, "Test prompt", []Skill{skill}, 10)

	// This line should not be reached due to the panic
	t.Fatal("Test should have panicked before reaching this point")
}

func Test_sanitizeToolName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "simple alphanumeric name",
			args: args{name: "getPetById"},
			want: "getPetById",
		},
		{
			name: "already with underscores",
			args: args{name: "get_pet_by_id"},
			want: "get_pet_by_id",
		},
		{
			name: "name with spaces",
			args: args{name: "get pet by id"},
			want: "get_pet_by_id",
		},
		{
			name: "name with special characters",
			args: args{name: "get-pet-by-id"},
			want: "get_pet_by_id",
		},
		{
			name: "name with multiple special characters",
			args: args{name: "get.pet@by#id"},
			want: "get_pet_by_id",
		},
		{
			name: "name with mixed case and special characters",
			args: args{name: "Get.Pet@By#Id"},
			want: "Get_Pet_By_Id",
		},
		{
			name: "name with numbers and special characters",
			args: args{name: "get-pet-123"},
			want: "get_pet_123",
		},
		{
			name: "name with consecutive special characters",
			args: args{name: "get--pet---by--id"},
			want: "get_pet_by_id",
		},
		{
			name: "name with leading/trailing special characters",
			args: args{name: "-get-pet-by-id-"},
			want: "_get_pet_by_id_",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeToolName(tt.args.name); got != tt.want {
				t.Errorf("SanitizeToolName() = %v, want %v", got, tt.want)
			}
		})
	}
}
