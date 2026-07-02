package server

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

func TestInjectAgentToolNudge(t *testing.T) {
	tool := []openai.Tool{{Type: openai.ToolTypeFunction}}
	models := []string{"glm", "qwen"}
	glm := "openrouter/z-ai/glm-4.6"

	// No tools: untouched.
	noTools := openai.ChatCompletionRequest{Model: glm, Messages: []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hi"},
	}}
	injectAgentToolNudge(&noTools, models)
	require.Len(t, noTools.Messages, 1)
	require.NotContains(t, noTools.Messages[0].Content, agentToolNudge)

	// Tool-enabled but unlisted model (Claude): untouched.
	nonMatch := openai.ChatCompletionRequest{Model: "claude-opus-4-8", Tools: tool, Messages: []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "You are helpful."},
	}}
	injectAgentToolNudge(&nonMatch, models)
	require.Len(t, nonMatch.Messages, 1)
	require.NotContains(t, nonMatch.Messages[0].Content, agentToolNudge)

	// Empty model list disables the nudge even for a listed-looking model.
	disabled := openai.ChatCompletionRequest{Model: glm, Tools: tool, Messages: []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "You are helpful."},
	}}
	injectAgentToolNudge(&disabled, nil)
	require.NotContains(t, disabled.Messages[0].Content, agentToolNudge)

	// Qwen, leading system message with plain content: appended in place (case-insensitive).
	withSys := openai.ChatCompletionRequest{Model: "Qwen/Qwen3-Coder", Tools: tool, Messages: []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "You are helpful."},
		{Role: openai.ChatMessageRoleUser, Content: "go"},
	}}
	injectAgentToolNudge(&withSys, models)
	require.Len(t, withSys.Messages, 2)
	require.Contains(t, withSys.Messages[0].Content, "You are helpful.")
	require.Contains(t, withSys.Messages[0].Content, agentToolNudge)

	// GLM, no system message: fresh one prepended.
	noSys := openai.ChatCompletionRequest{Model: "GLM 5.2", Tools: tool, Messages: []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "go"},
	}}
	injectAgentToolNudge(&noSys, models)
	require.Len(t, noSys.Messages, 2)
	require.Equal(t, openai.ChatMessageRoleSystem, noSys.Messages[0].Role)
	require.Equal(t, agentToolNudge, noSys.Messages[0].Content)
	require.Equal(t, openai.ChatMessageRoleUser, noSys.Messages[1].Role)

	// GLM, system message using MultiContent: prepend, never set both fields.
	multi := openai.ChatCompletionRequest{Model: glm, Tools: tool, Messages: []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, MultiContent: []openai.ChatMessagePart{{Type: openai.ChatMessagePartTypeText, Text: "sys"}}},
	}}
	injectAgentToolNudge(&multi, models)
	require.Len(t, multi.Messages, 2)
	require.Equal(t, agentToolNudge, multi.Messages[0].Content)
	require.Empty(t, multi.Messages[0].MultiContent)
	require.Empty(t, multi.Messages[1].Content)
}
