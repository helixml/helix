package server

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

func TestInjectAgentToolNudge(t *testing.T) {
	tool := []openai.Tool{{Type: openai.ToolTypeFunction}}

	// No tools: untouched.
	noTools := openai.ChatCompletionRequest{Messages: []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hi"},
	}}
	injectAgentToolNudge(&noTools)
	require.Len(t, noTools.Messages, 1)
	require.NotContains(t, noTools.Messages[0].Content, agentToolNudge)

	// Leading system message with plain content: appended in place.
	withSys := openai.ChatCompletionRequest{Tools: tool, Messages: []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "You are helpful."},
		{Role: openai.ChatMessageRoleUser, Content: "go"},
	}}
	injectAgentToolNudge(&withSys)
	require.Len(t, withSys.Messages, 2)
	require.Contains(t, withSys.Messages[0].Content, "You are helpful.")
	require.Contains(t, withSys.Messages[0].Content, agentToolNudge)

	// No system message: fresh one prepended.
	noSys := openai.ChatCompletionRequest{Tools: tool, Messages: []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "go"},
	}}
	injectAgentToolNudge(&noSys)
	require.Len(t, noSys.Messages, 2)
	require.Equal(t, openai.ChatMessageRoleSystem, noSys.Messages[0].Role)
	require.Equal(t, agentToolNudge, noSys.Messages[0].Content)
	require.Equal(t, openai.ChatMessageRoleUser, noSys.Messages[1].Role)

	// System message using MultiContent: prepend, never set both fields.
	multi := openai.ChatCompletionRequest{Tools: tool, Messages: []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, MultiContent: []openai.ChatMessagePart{{Type: openai.ChatMessagePartTypeText, Text: "sys"}}},
	}}
	injectAgentToolNudge(&multi)
	require.Len(t, multi.Messages, 2)
	require.Equal(t, agentToolNudge, multi.Messages[0].Content)
	require.Empty(t, multi.Messages[0].MultiContent)
	require.Empty(t, multi.Messages[1].Content)
}
