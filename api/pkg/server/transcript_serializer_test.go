package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/server/wsprotocol"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustEntries(t *testing.T, entries []wsprotocol.ResponseEntry) []byte {
	t.Helper()
	raw, err := json.Marshal(entries)
	require.NoError(t, err)
	return raw
}

func TestSerializeTranscript_EmptyInput(t *testing.T) {
	assert.Equal(t, "", serializeTranscript(nil, maxTranscriptBytes))
	assert.Equal(t, "", serializeTranscript([]*types.Interaction{}, maxTranscriptBytes))
}

func TestSerializeTranscript_SkipsForkSeedAndIncomplete(t *testing.T) {
	interactions := []*types.Interaction{
		{
			Trigger:         types.InteractionTriggerForkSeed,
			State:           types.InteractionStateComplete,
			PromptMessage:   "seed prompt",
			ResponseMessage: "seed response (should not appear)",
		},
		{
			State:           types.InteractionStateWaiting,
			PromptMessage:   "in-flight user message",
			ResponseMessage: "partial response (should not appear)",
		},
		{
			State:           types.InteractionStateComplete,
			PromptMessage:   "hello",
			ResponseMessage: "hi there",
		},
	}

	got := serializeTranscript(interactions, maxTranscriptBytes)

	assert.NotContains(t, got, "seed prompt")
	assert.NotContains(t, got, "seed response")
	assert.NotContains(t, got, "in-flight user message")
	assert.NotContains(t, got, "partial response")
	assert.Contains(t, got, "**User:** hello")
	assert.Contains(t, got, "**Assistant:** hi there")
}

func TestSerializeTranscript_StructuredResponseEntries(t *testing.T) {
	entries := []wsprotocol.ResponseEntry{
		{Type: "text", Content: "Let me check the file.", MessageID: "1"},
		{Type: "tool_call", Content: "/etc/hosts", MessageID: "2", ToolName: "read_file", ToolStatus: "Completed"},
		{Type: "text", Content: "It's the standard hosts file.", MessageID: "3"},
	}
	interactions := []*types.Interaction{
		{
			State:           types.InteractionStateComplete,
			PromptMessage:   "what's in /etc/hosts?",
			ResponseEntries: mustEntries(t, entries),
			// ResponseMessage intentionally empty to prove entries take precedence
		},
	}

	got := serializeTranscript(interactions, maxTranscriptBytes)

	assert.Contains(t, got, "Let me check the file.")
	assert.Contains(t, got, "[read_file: Completed]")
	assert.Contains(t, got, "/etc/hosts")
	assert.Contains(t, got, "It's the standard hosts file.")
}

func TestSerializeTranscript_FallsBackToResponseMessageWhenEntriesAbsent(t *testing.T) {
	interactions := []*types.Interaction{
		{
			State:           types.InteractionStateComplete,
			PromptMessage:   "old session",
			ResponseMessage: "legacy flat response",
		},
	}

	got := serializeTranscript(interactions, maxTranscriptBytes)

	assert.Contains(t, got, "**User:** old session")
	assert.Contains(t, got, "**Assistant:** legacy flat response")
}

func TestSerializeTranscript_TruncatesFromFrontWhenOverBudget(t *testing.T) {
	// Three blocks of ~5000 bytes; cap at 12000 → oldest one should be dropped.
	bigText := strings.Repeat("x", 5000)
	mk := func(prompt, response string) *types.Interaction {
		return &types.Interaction{
			State:           types.InteractionStateComplete,
			PromptMessage:   prompt,
			ResponseMessage: response,
		}
	}
	interactions := []*types.Interaction{
		mk("oldest prompt", bigText+" OLDEST_MARKER"),
		mk("middle prompt", bigText+" MIDDLE_MARKER"),
		mk("newest prompt", bigText+" NEWEST_MARKER"),
	}

	got := serializeTranscript(interactions, 12_000)

	assert.True(t, strings.HasPrefix(got, transcriptTruncationNotice),
		"transcript should be prefixed with truncation notice")
	assert.NotContains(t, got, "OLDEST_MARKER", "oldest block should have been dropped")
	assert.Contains(t, got, "MIDDLE_MARKER")
	assert.Contains(t, got, "NEWEST_MARKER")
	assert.LessOrEqual(t, len(got), 12_000, "truncated transcript must fit budget")
}

func TestSerializeTranscript_NoTruncationWhenUnderBudget(t *testing.T) {
	interactions := []*types.Interaction{
		{State: types.InteractionStateComplete, PromptMessage: "small", ResponseMessage: "tiny"},
	}

	got := serializeTranscript(interactions, maxTranscriptBytes)

	assert.NotContains(t, got, transcriptTruncationNotice)
}

func TestSerializeAgentResponse_PrefersStructuredEntries(t *testing.T) {
	entries := []wsprotocol.ResponseEntry{
		{Type: "text", Content: "structured text", MessageID: "1"},
	}
	in := &types.Interaction{
		ResponseEntries: mustEntries(t, entries),
		ResponseMessage: "legacy fallback should be ignored",
	}

	got := serializeAgentResponse(in)

	assert.Equal(t, "structured text", got)
}

func TestSerializeAgentResponse_FallsBackOnMalformedEntries(t *testing.T) {
	in := &types.Interaction{
		ResponseEntries: []byte(`{"not": "an array"}`),
		ResponseMessage: "legacy fallback",
	}

	got := serializeAgentResponse(in)

	assert.Equal(t, "legacy fallback", got)
}

func TestSerializeAgentResponse_ToolCallWithoutStatus(t *testing.T) {
	entries := []wsprotocol.ResponseEntry{
		{Type: "tool_call", Content: "cmd", MessageID: "1", ToolName: "bash"},
	}
	in := &types.Interaction{ResponseEntries: mustEntries(t, entries)}

	got := serializeAgentResponse(in)

	assert.Contains(t, got, "[bash]")
	assert.Contains(t, got, "cmd")
}

func TestFindForkSeed(t *testing.T) {
	seed := &types.Interaction{ID: "seed1", Trigger: types.InteractionTriggerForkSeed}
	interactions := []*types.Interaction{
		{ID: "i1", Trigger: ""},
		seed,
		{ID: "i2", Trigger: ""},
	}
	assert.Same(t, seed, findForkSeed(interactions))
	assert.Nil(t, findForkSeed(nil))
	assert.Nil(t, findForkSeed([]*types.Interaction{{Trigger: ""}}))
}
