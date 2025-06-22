package skill

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestCalculatorTool_Execute(t *testing.T) {
	tool := &CalculatorTool{}
	meta := agent.Meta{}
	ctx := context.Background()

	testCases := []struct {
		name       string
		expression interface{}
		expected   string
		expectErr  bool
	}{
		{
			name:       "simple addition",
			expression: "2 + 2",
			expected:   "4",
		},
		{
			name:       "simple subtraction",
			expression: "5 - 3",
			expected:   "2",
		},
		{
			name:       "simple multiplication",
			expression: "3 * 7",
			expected:   "21",
		},
		{
			name:       "simple division",
			expression: "10 / 2",
			expected:   "5",
		},
		{
			name:       "integer float result",
			expression: "2.5 * 2",
			expected:   "5",
		},
		{
			name:       "float result",
			expression: "1.2 + 3.4",
			expected:   "4.6",
		},
		{
			name:       "math sqrt",
			expression: "Math.sqrt(16)",
			expected:   "4",
		},
		{
			name:       "math pow",
			expression: "Math.pow(2, 3)",
			expected:   "8",
		},
		{
			name:       "parentheses",
			expression: "10 * (5 + 3)",
			expected:   "80",
		},
		{
			name:       "division by zero",
			expression: "1 / 0",
			expected:   "+Inf",
		},
		{
			name:       "invalid expression",
			expression: "2 +",
			expectErr:  true,
		},
		{
			name:       "empty expression",
			expression: "",
			expectErr:  true,
		},
		{
			name:       "not a string",
			expression: 123,
			expectErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]interface{}{
				"expression": tc.expression,
			}
			result, err := tool.Execute(ctx, meta, args)

			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, result)
			}
		})
	}

	t.Run("missing expression", func(t *testing.T) {
		args := map[string]interface{}{}
		_, err := tool.Execute(ctx, meta, args)
		require.Error(t, err)
	})
}
