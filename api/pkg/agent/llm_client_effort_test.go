package agent

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
)

// The "none" reasoning effort is overloaded: for models that predate the value
// it means "don't send the parameter", but for GPT-5.x it is a real value that
// MUST be sent — omitting it makes the provider apply its default ("medium"),
// which gpt-5.6-* rejects when the request carries function tools.
func TestApplyReasoningEffort(t *testing.T) {
	tests := []struct {
		name        string
		acceptNone  bool
		configured  string
		paramEffort string
		want        string
	}{
		{"none stripped for models that do not accept it", false, "", "none", ""},
		{"none sent for models that accept it", true, "", "none", "none"},
		{"other efforts pass through unchanged", false, "", "medium", "medium"},
		{"other efforts pass through on none-capable models", true, "", "high", "high"},
		{"empty stays empty when nothing configured", false, "", "", ""},
		// decideNextAction builds its request without an effort, so the model's
		// configured value has to be filled in — otherwise the parameter is
		// omitted and the provider applies its own default.
		{"configured effort fills an empty param", true, "none", "", "none"},
		{"configured effort fills an empty param (non-none)", false, "medium", "", "medium"},
		{"configured none still stripped when unsupported", false, "none", "", ""},
		{"explicit param wins over configured", true, "none", "high", "high"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := applyReasoningEffort(
				&LLMModelConfig{
					AcceptsNoneReasoningEffort: tc.acceptNone,
					ReasoningEffort:            tc.configured,
				},
				openai.ChatCompletionRequest{ReasoningEffort: tc.paramEffort},
			)
			assert.Equal(t, tc.want, got.ReasoningEffort)
		})
	}
}

func TestApplyReasoningEffort_NilModelStrips(t *testing.T) {
	got := applyReasoningEffort(nil, openai.ChatCompletionRequest{ReasoningEffort: "none"})
	assert.Equal(t, "", got.ReasoningEffort, "unknown model must fall back to the historical strip")
}
