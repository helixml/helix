package agent

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

func Test_getUniqueToolCalls(t *testing.T) {
	type args struct {
		toolCalls []openai.ToolCall
	}
	tests := []struct {
		name string
		args args
		want []openai.ToolCall
	}{
		{
			name: "empty input",
			args: args{
				toolCalls: []openai.ToolCall{},
			},
			want: []openai.ToolCall{},
		},
		{
			name: "single tool call",
			args: args{
				toolCalls: []openai.ToolCall{
					{
						ID: "call_1",
						Function: openai.FunctionCall{
							Name:      "test_function",
							Arguments: `{"param": "value"}`,
						},
					},
				},
			},
			want: []openai.ToolCall{
				{
					ID: "call_1",
					Function: openai.FunctionCall{
						Name:      "test_function",
						Arguments: `{"param": "value"}`,
					},
				},
			},
		},
		{
			name: "duplicate tool calls with same name and arguments",
			args: args{
				toolCalls: []openai.ToolCall{
					{
						ID: "call_1",
						Function: openai.FunctionCall{
							Name:      "test_function",
							Arguments: `{"param": "value"}`,
						},
					},
					{
						ID: "call_2",
						Function: openai.FunctionCall{
							Name:      "test_function",
							Arguments: `{"param": "value"}`,
						},
					},
				},
			},
			want: []openai.ToolCall{
				{
					ID: "call_1",
					Function: openai.FunctionCall{
						Name:      "test_function",
						Arguments: `{"param": "value"}`,
					},
				},
			},
		},
		{
			name: "different function names",
			args: args{
				toolCalls: []openai.ToolCall{
					{
						ID: "call_1",
						Function: openai.FunctionCall{
							Name:      "function_a",
							Arguments: `{"param": "value"}`,
						},
					},
					{
						ID: "call_2",
						Function: openai.FunctionCall{
							Name:      "function_b",
							Arguments: `{"param": "value"}`,
						},
					},
				},
			},
			want: []openai.ToolCall{
				{
					ID: "call_1",
					Function: openai.FunctionCall{
						Name:      "function_a",
						Arguments: `{"param": "value"}`,
					},
				},
				{
					ID: "call_2",
					Function: openai.FunctionCall{
						Name:      "function_b",
						Arguments: `{"param": "value"}`,
					},
				},
			},
		},
		{
			name: "same function name but different arguments",
			args: args{
				toolCalls: []openai.ToolCall{
					{
						ID: "call_1",
						Function: openai.FunctionCall{
							Name:      "test_function",
							Arguments: `{"param": "value1"}`,
						},
					},
					{
						ID: "call_2",
						Function: openai.FunctionCall{
							Name:      "test_function",
							Arguments: `{"param": "value2"}`,
						},
					},
				},
			},
			want: []openai.ToolCall{
				{
					ID: "call_1",
					Function: openai.FunctionCall{
						Name:      "test_function",
						Arguments: `{"param": "value1"}`,
					},
				},
				{
					ID: "call_2",
					Function: openai.FunctionCall{
						Name:      "test_function",
						Arguments: `{"param": "value2"}`,
					},
				},
			},
		},
		{
			name: "complex case with multiple duplicates",
			args: args{
				toolCalls: []openai.ToolCall{
					{
						ID: "call_1",
						Function: openai.FunctionCall{
							Name:      "function_a",
							Arguments: `{"param": "value1"}`,
						},
					},
					{
						ID: "call_2",
						Function: openai.FunctionCall{
							Name:      "function_a",
							Arguments: `{"param": "value1"}`,
						},
					},
					{
						ID: "call_3",
						Function: openai.FunctionCall{
							Name:      "function_b",
							Arguments: `{"param": "value2"}`,
						},
					},
					{
						ID: "call_4",
						Function: openai.FunctionCall{
							Name:      "function_a",
							Arguments: `{"param": "value1"}`,
						},
					},
					{
						ID: "call_5",
						Function: openai.FunctionCall{
							Name:      "function_b",
							Arguments: `{"param": "value2"}`,
						},
					},
				},
			},
			want: []openai.ToolCall{
				{
					ID: "call_1",
					Function: openai.FunctionCall{
						Name:      "function_a",
						Arguments: `{"param": "value1"}`,
					},
				},
				{
					ID: "call_3",
					Function: openai.FunctionCall{
						Name:      "function_b",
						Arguments: `{"param": "value2"}`,
					},
				},
			},
		},
		{
			name: "empty function name and arguments",
			args: args{
				toolCalls: []openai.ToolCall{
					{
						ID: "call_1",
						Function: openai.FunctionCall{
							Name:      "",
							Arguments: "",
						},
					},
					{
						ID: "call_2",
						Function: openai.FunctionCall{
							Name:      "",
							Arguments: "",
						},
					},
				},
			},
			want: []openai.ToolCall{
				{
					ID: "call_1",
					Function: openai.FunctionCall{
						Name:      "",
						Arguments: "",
					},
				},
			},
		},
		{
			name: "nil input",
			args: args{
				toolCalls: nil,
			},
			want: []openai.ToolCall{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getUniqueToolCalls(tt.args.toolCalls)
			assert.Equal(t, tt.want, got)
		})
	}
}
