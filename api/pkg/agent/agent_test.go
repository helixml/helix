package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	helix_openai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

func TestAgent(t *testing.T) {
	suite.Run(t, new(AgentTestSuite))
}

type AgentTestSuite struct {
	suite.Suite
	ctrl                     *gomock.Controller
	generationalOpenaiClient *helix_openai.MockClient
	reasoningOpenaiClient    *helix_openai.MockClient
	llm                      *LLM
}

func (s *AgentTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())

	s.generationalOpenaiClient = helix_openai.NewMockClient(s.ctrl)
	s.reasoningOpenaiClient = helix_openai.NewMockClient(s.ctrl)

	s.llm = NewLLM(
		&LLMModelConfig{
			Client: s.reasoningOpenaiClient,
			Model:  "gpt-4o-mini",
		},
		&LLMModelConfig{
			Client: s.generationalOpenaiClient,
			Model:  "gpt-4o-mini",
		},
		&LLMModelConfig{
			Client: s.reasoningOpenaiClient,
			Model:  "gpt-4o-mini",
		},
		&LLMModelConfig{
			Client: s.generationalOpenaiClient,
			Model:  "gpt-4o-mini",
		},
	)
}

func (s *AgentTestSuite) Test_Agent_NoSkills() {
	agent := NewAgent(NewLogStepInfoEmitter(), "Test prompt", []Skill{}, 10)

	respCh := make(chan Response)

	// Should be direct call to LLM
	s.generationalOpenaiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).Return(openai.ChatCompletionResponse{
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
		}, &MemoryBlock{}, &MemoryBlock{}, respCh, false)
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

func (s *AgentTestSuite) Test_Agent_NoSkills_Conversational() {
	agent := NewAgent(NewLogStepInfoEmitter(), "Test prompt", []Skill{}, 10)

	respCh := make(chan Response)

	stream, writer, err := helix_openai.NewOpenAIStreamingAdapter(openai.ChatCompletionRequest{})
	s.Require().NoError(err)

	// Should be direct call to LLM
	s.generationalOpenaiClient.EXPECT().CreateChatCompletionStream(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
			return stream, nil
		})

	go func() {
		defer writer.Close()

		for i := 0; i < 3; i++ {
			// Create a chat completion chunk and encode it to json
			chunk := openai.ChatCompletionStreamResponse{
				ID:     "chatcmpl-123",
				Object: "chat.completion.chunk",
				Model:  "mistralai/Mistral-7B-Instruct-v0.1",
				Choices: []openai.ChatCompletionStreamChoice{
					{
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Content: "Test response",
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
			s.Require().NoError(err)

			err = writeChunk(writer, bts)
			s.Require().NoError(err)

			// _, err = writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(bts))))
			// suite.NoError(err)
		}

		_, err = writer.Write([]byte("[DONE]"))
		s.Require().NoError(err)
	}()

	go func() {
		defer close(respCh)

		agent.Run(context.Background(), Meta{}, s.llm, &MessageList{
			Messages: []*openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "Test question",
				},
			},
		}, &MemoryBlock{}, &MemoryBlock{}, respCh, true)
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

func (s *AgentTestSuite) Test_Agent_NoSkills_PlainTextKnowledge() {
	agent := NewAgent(NewLogStepInfoEmitter(), "Test prompt", []Skill{}, 10)

	respCh := make(chan Response)

	stream, writer, err := helix_openai.NewOpenAIStreamingAdapter(openai.ChatCompletionRequest{})
	s.Require().NoError(err)

	// Should be direct call to LLM
	s.generationalOpenaiClient.EXPECT().CreateChatCompletionStream(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
			// We should have the knowledge block in the messages
			s.Require().Equal(openai.ChatMessageRoleDeveloper, req.Messages[0].Role)
			s.Require().Contains(req.Messages[0].Content, "Test knowledge")
			s.Require().Contains(req.Messages[0].Content, "Test description")
			s.Require().Contains(req.Messages[0].Content, "Test contents")

			return stream, nil
		})

	knowledgeBlock := NewMemoryBlock()
	knowledgeBlock.AddString("name", "Test knowledge")
	knowledgeBlock.AddString("description", "Test description")
	knowledgeBlock.AddString("contents", "Test contents")

	go func() {
		defer writer.Close()

		for i := 0; i < 3; i++ {
			// Create a chat completion chunk and encode it to json
			chunk := openai.ChatCompletionStreamResponse{
				ID:     "chatcmpl-123",
				Object: "chat.completion.chunk",
				Model:  "mistralai/Mistral-7B-Instruct-v0.1",
				Choices: []openai.ChatCompletionStreamChoice{
					{
						Delta: openai.ChatCompletionStreamChoiceDelta{
							Content: "Test response",
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
			s.Require().NoError(err)

			err = writeChunk(writer, bts)
			s.Require().NoError(err)

			// _, err = writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(bts))))
			// suite.NoError(err)
		}

		_, err = writer.Write([]byte("[DONE]"))
		s.Require().NoError(err)
	}()

	go func() {
		defer close(respCh)

		agent.Run(context.Background(), Meta{}, s.llm, &MessageList{
			Messages: []*openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "Test question",
				},
			},
		}, &MemoryBlock{}, knowledgeBlock, respCh, true)
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

func (s *AgentTestSuite) Test_Agent_DirectSkill() {

	tool := &CurrencyConverter{
		toolName:    "CurrencyConverter",
		description: "Convert currency",
	}
	skill := Skill{
		Name:          "CurrencyConverter",
		Description:   "Convert currency",
		Direct:        true,
		ProcessOutput: true,
		Tools: []Tool{
			tool,
		},
	}

	knowledgeBlock := NewMemoryBlock()
	knowledgeBlock.AddString("name", "Test knowledge")
	knowledgeBlock.AddString("description", "Test description")
	knowledgeBlock.AddString("contents", "Test contents")

	agent := NewAgent(NewLogStepInfoEmitter(), "Convert 100 USD to EUR", []Skill{skill}, 10)

	respCh := make(chan Response)

	// First call should be about deciding next action
	s.generationalOpenaiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			// Check step from the context, should be "decide"
			step, ok := helix_openai.GetStep(ctx)
			s.Require().True(ok)
			s.Require().Equal("decide_next_action", string(step.Step))

			// Check request
			s.Require().Equal(s.llm.GenerationModel.Model, req.Model)
			s.Require().Equal(openai.ChatMessageRoleDeveloper, req.Messages[0].Role)
			s.Require().Contains(req.Messages[0].Content, "You can use skill functions to help answer the user's question effectively")

			// Second message should be the user question
			s.Require().Equal(openai.ChatMessageRoleUser, req.Messages[1].Role)
			s.Require().Equal("Convert 100 USD to EUR", req.Messages[1].Content)

			// Should return a tool call
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Role: openai.ChatMessageRoleAssistant,
							ToolCalls: []openai.ToolCall{
								{
									ID: "tool_call_id",
									Function: openai.FunctionCall{
										Name:      "CurrencyConverter",
										Arguments: `{"from_currency": "USD", "to_currency": "EUR"}`,
									},
								},
							},
						},
					},
				},
			}, nil
		})

	// Then we should call the skill
	s.generationalOpenaiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			// Check step from the context, should be "decide"
			step, ok := helix_openai.GetStep(ctx)
			s.Require().True(ok)
			s.Require().Equal("decide_next_action", string(step.Step))

			// Last message should be role tool
			s.Require().Equal(openai.ChatMessageRoleTool, req.Messages[len(req.Messages)-1].Role)
			// It should have our mocked response
			s.Require().Equal("100 USD is 80 EUR", req.Messages[len(req.Messages)-1].Content)

			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Role:    openai.ChatMessageRoleAssistant,
							Content: "100 USD is 80 EUR",
							ToolCalls: []openai.ToolCall{
								{
									ID: "tool_call_id_2",
									Function: openai.FunctionCall{
										Name:      "stop",
										Arguments: `{"callSummarizer": false}`,
									},
								},
							},
						},
					},
				},
			}, nil
		})

	// Should be one more to summarize

	s.generationalOpenaiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			// Check step from the context, should be "decide"
			step, ok := helix_openai.GetStep(ctx)
			s.Require().True(ok)
			s.Require().Equal("summarize_multiple_tool_results", string(step.Step))

			// Should have tool role call with results
			s.Require().Equal(openai.ChatMessageRoleTool, req.Messages[len(req.Messages)-1].Role)
			s.Require().Equal("100 USD is 80 EUR", req.Messages[len(req.Messages)-1].Content)

			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Role:    openai.ChatMessageRoleAssistant,
							Content: "100 USD is 80 EUR",
							ToolCalls: []openai.ToolCall{
								{
									ID: "tool_call_id_2",
									Function: openai.FunctionCall{
										Name:      "stop",
										Arguments: `{"callSummarizer": false}`,
									},
								},
							},
						},
					},
				},
			}, nil
		})

	go func() {
		defer close(respCh)

		// Recover
		defer func() {
			if r := recover(); r != nil {
				s.Require().Fail(fmt.Sprintf("Agent panicked: %v", r))
			}
		}()

		agent.Run(context.Background(), Meta{}, s.llm, &MessageList{
			Messages: []*openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "Convert 100 USD to EUR",
				},
			},
		}, &MemoryBlock{}, knowledgeBlock, respCh, false)
	}()

	var aggregatedResponse string

	for response := range respCh {
		// IF we get an error, fail
		if response.Type == ResponseTypeError {
			s.Require().Fail(response.Content)
		}

		if response.Type == ResponseTypePartialText {
			aggregatedResponse += response.Content

		}
		if response.Type == ResponseTypeEnd {
			break
		}
	}

	s.Require().Equal("100 USD is 80 EUR", aggregatedResponse)
	s.Require().Equal(1, tool.callCount)
}

func (s *AgentTestSuite) Test_Agent_DirectSkill_MultipleSkillsUsed() {

	browserTool := NewMockTool(s.ctrl)
	browserTool.EXPECT().Name().Return("Browser").AnyTimes()
	browserTool.EXPECT().Description().Return("Browse the web").AnyTimes()
	browserTool.EXPECT().StatusMessage().Return("Browsing the web").AnyTimes()
	browserTool.EXPECT().Icon().Return("").AnyTimes()
	browserTool.EXPECT().OpenAI().Return([]openai.Tool{
		{
			Type: openai.ToolTypeFunction,
		}}).AnyTimes()

	browserSkill := Skill{
		Name:          "Browser",
		Description:   "Browse the web",
		Direct:        true,
		ProcessOutput: true,
		Tools: []Tool{
			browserTool,
		},
	}

	agent := NewAgent(NewLogStepInfoEmitter(), "Whats on the news?", []Skill{browserSkill}, 10)
	respCh := make(chan Response)

	// First call should be about deciding next action
	s.generationalOpenaiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			// Check step from the context, should be "decide"
			step, ok := helix_openai.GetStep(ctx)
			s.Require().True(ok)
			s.Require().Equal("decide_next_action", string(step.Step))

			// Check request
			s.Require().Equal(s.llm.GenerationModel.Model, req.Model)
			s.Require().Equal(openai.ChatMessageRoleDeveloper, req.Messages[0].Role)
			// Should have our skill selection prompt
			s.Require().Contains(req.Messages[0].Content, "You can use skill functions to help answer the user's question effectively")

			// Second message should be the user question
			s.Require().Equal(openai.ChatMessageRoleUser, req.Messages[1].Role)
			s.Require().Equal("Whats on the news?", req.Messages[1].Content)

			// Should return a tool call
			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Role: openai.ChatMessageRoleAssistant,
							ToolCalls: []openai.ToolCall{
								{
									ID: "tool_call_id_google",
									Function: openai.FunctionCall{
										Name:      "Browser",
										Arguments: `{"url": "https://www.google.com"}`,
									},
								},
								{
									ID: "tool_call_id_bing",
									Function: openai.FunctionCall{
										Name:      "Browser",
										Arguments: `{"url": "https://bing.com"}`,
									},
								},
							},
						},
					},
				},
			}, nil
		})

	// Then we should call the skills
	browserTool.EXPECT().Execute(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ Meta, args map[string]interface{}) (string, error) {
			url, ok := args["url"].(string)
			s.Require().True(ok)

			if url == "https://www.google.com" {
				return "Google Search Results", nil
			}

			if url == "https://bing.com" {
				return "Bing Search Results", nil
			}

			s.Require().Fail("Unexpected url: %s", url)

			return "", fmt.Errorf("unexpected url: %s", url)
		},
	).Times(2)

	s.generationalOpenaiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			// Check step from the context, should be "decide"
			step, ok := helix_openai.GetStep(ctx)
			s.Require().True(ok)
			s.Require().Equal("decide_next_action", string(step.Step))

			// Go through tools and find both google and bing results
			s.Require().Contains(req.Messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    "Google Search Results",
				ToolCallID: "tool_call_id_google",
			})

			s.Require().Contains(req.Messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    "Bing Search Results",
				ToolCallID: "tool_call_id_bing",
			})

			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Role: openai.ChatMessageRoleAssistant,
							ToolCalls: []openai.ToolCall{
								{
									ID: "tool_call_id_2",
									Function: openai.FunctionCall{
										Name:      "stop",
										Arguments: `{"callSummarizer": false}`,
									},
								},
							},
						},
					},
				},
			}, nil
		})

	// Should be one more to summarize

	s.generationalOpenaiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
			// Check step from the context, should be "decide"
			step, ok := helix_openai.GetStep(ctx)
			s.Require().True(ok)
			s.Require().Equal("summarize_multiple_tool_results", string(step.Step))

			// Go through tools and find both google and bing results
			s.Require().Contains(req.Messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    "Google Search Results",
				ToolCallID: "tool_call_id_google",
			})

			s.Require().Contains(req.Messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    "Bing Search Results",
				ToolCallID: "tool_call_id_bing",
			})

			return openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Role:    openai.ChatMessageRoleAssistant,
							Content: "<summarized news here>",
							ToolCalls: []openai.ToolCall{
								{
									ID: "tool_call_id_2",
									Function: openai.FunctionCall{
										Name:      "stop",
										Arguments: `{"callSummarizer": false}`,
									},
								},
							},
						},
					},
				},
			}, nil
		})

	go func() {
		defer close(respCh)

		// Recover
		defer func() {
			if r := recover(); r != nil {
				s.Require().Fail(fmt.Sprintf("Agent panicked: %v", r))
			}
		}()

		agent.Run(context.Background(), Meta{}, s.llm, &MessageList{
			Messages: []*openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "Whats on the news?",
				},
			},
		}, &MemoryBlock{}, &MemoryBlock{}, respCh, false)
	}()

	var aggregatedResponse string

	for response := range respCh {
		// IF we get an error, fail
		if response.Type == ResponseTypeError {
			s.Require().Fail(response.Content)
		}

		if response.Type == ResponseTypePartialText {
			aggregatedResponse += response.Content

		}
		if response.Type == ResponseTypeEnd {
			break
		}
	}

	s.Require().Equal("<summarized news here>", aggregatedResponse)
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

type CurrencyConverter struct {
	toolName    string
	description string

	callCount int // How many times the tool was called (Execute func)
}

var _ Tool = &CurrencyConverter{}

func (b *CurrencyConverter) Name() string {
	return b.toolName
}

func (b *CurrencyConverter) String() string {
	return b.toolName
}

func (b *CurrencyConverter) Description() string {
	return b.description
}

func (b *CurrencyConverter) StatusMessage() string {
	return "Converting currency"
}

func (b *CurrencyConverter) Icon() string {
	return ""
}

func (b *CurrencyConverter) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        b.toolName,
				Description: b.description,
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"from_currency": {
							Type:        jsonschema.String,
							Description: "From currency",
						},
						"to_currency": {
							Type:        jsonschema.String,
							Description: "To currency",
						},
					},
					Required: []string{"from_currency", "to_currency"},
				},
			},
		},
	}
}

func (b *CurrencyConverter) Execute(_ context.Context, _ Meta, _ map[string]interface{}) (string, error) {
	b.callCount++
	return "100 USD is 80 EUR", nil
}

func writeChunk(w io.Writer, chunk []byte) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", string(chunk))
	if err != nil {
		return fmt.Errorf("error writing chunk '%s': %w", string(chunk), err)
	}

	// Flush the ResponseWriter buffer to send the chunk immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	} else {
		log.Warn().Msg("ResponseWriter does not support Flusher interface")
	}

	return nil
}
