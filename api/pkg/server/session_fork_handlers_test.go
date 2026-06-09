package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newForkTestServer builds a minimal server backed by the in-memory store and
// pubsub. externalAgentExecutor is intentionally left nil — the fork helper
// is expected to handle that gracefully (the desktop provisioning step logs
// and continues so the rest of the flow is testable without spinning up a
// container).
func newForkTestServer(t *testing.T) (*HelixAPIServer, *memorystore.MemoryStore) {
	t.Helper()
	mem := memorystore.New()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	srv := NewTestServer(mem, ps)
	return srv, mem
}

func newTestParentSession(owner string) *types.Session {
	now := time.Now().Add(-1 * time.Hour)
	return &types.Session{
		ID:        "ses_parent_" + randSuffix(),
		Name:      "Original chat",
		Owner:     owner,
		OwnerType: types.OwnerTypeUser,
		Type:      types.SessionTypeText,
		Mode:      types.SessionModeInference,
		Provider:  "anthropic",
		ModelName: "claude-opus-4-7",
		ProjectID: "prj_test",
		ParentApp: "app_parent",
		Created:   now,
		Updated:   now,
		Metadata: types.SessionMetadata{
			AgentType:        "zed_external",
			CodeAgentRuntime: types.CodeAgentRuntimeClaudeCode,
			ZedAgentName:     "claude",
			SystemPrompt:     "be helpful",
			SpecTaskID:       "task_test",
			SessionRole:      "planning",
		},
	}
}

var forkTestCounter int

func randSuffix() string {
	forkTestCounter++
	return time.Now().Format("150405") + "_" + strings.TrimSpace(itoaInt(forkTestCounter))
}

func itoaInt(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func seedParentWithInteractions(t *testing.T, mem *memorystore.MemoryStore, parent *types.Session, n int) []*types.Interaction {
	t.Helper()
	ctx := context.Background()
	_, err := mem.CreateSession(ctx, *parent)
	require.NoError(t, err)
	out := make([]*types.Interaction, 0, n)
	for i := 0; i < n; i++ {
		in := &types.Interaction{
			Created:         time.Now().Add(time.Duration(-n+i) * time.Minute),
			Updated:         time.Now(),
			SessionID:       parent.ID,
			UserID:          parent.Owner,
			GenerationID:    parent.GenerationID,
			Mode:            types.SessionModeInference,
			State:           types.InteractionStateComplete,
			PromptMessage:   "user turn " + itoaInt(i),
			ResponseMessage: "agent reply " + itoaInt(i),
		}
		created, err := mem.CreateInteraction(ctx, in)
		require.NoError(t, err)
		out = append(out, created)
	}
	return out
}

func TestResolveForkTarget_ExplicitRuntimeWins(t *testing.T) {
	srv, _ := newForkTestServer(t)
	parent := newTestParentSession("user_a")

	runtime, appID, err := srv.resolveForkTarget(context.Background(), parent, ForkSessionRequest{
		CodeAgentRuntime: types.CodeAgentRuntimeQwenCode,
		HelixAppID:       "app_other",
	})
	require.NoError(t, err)
	assert.Equal(t, types.CodeAgentRuntimeQwenCode, runtime)
	assert.Equal(t, "app_other", appID)
}

func TestResolveForkTarget_ResolveFromHelixApp(t *testing.T) {
	srv, mem := newForkTestServer(t)
	parent := newTestParentSession("user_a")
	mem.SeedApp(&types.App{
		ID: "app_target",
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						AgentType:        types.AgentTypeZedExternal,
						CodeAgentRuntime: types.CodeAgentRuntimeGeminiCLI,
					},
				},
			},
		},
	})

	runtime, appID, err := srv.resolveForkTarget(context.Background(), parent, ForkSessionRequest{HelixAppID: "app_target"})
	require.NoError(t, err)
	assert.Equal(t, types.CodeAgentRuntimeGeminiCLI, runtime)
	assert.Equal(t, "app_target", appID)
}

func TestResolveForkTarget_AppMissingZedExternalAssistant(t *testing.T) {
	srv, mem := newForkTestServer(t)
	parent := newTestParentSession("user_a")
	mem.SeedApp(&types.App{
		ID: "app_no_zed",
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{AgentType: types.AgentTypeHelixBasic}},
			},
		},
	})

	_, _, err := srv.resolveForkTarget(context.Background(), parent, ForkSessionRequest{HelixAppID: "app_no_zed"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no zed_external assistant")
}

func TestResolveForkTarget_AppNotFound(t *testing.T) {
	srv, _ := newForkTestServer(t)
	parent := newTestParentSession("user_a")

	_, _, err := srv.resolveForkTarget(context.Background(), parent, ForkSessionRequest{HelixAppID: "app_missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to look up target app")
}

func TestForkSessionFromParent_SnapshotsTranscriptAndPausesParent(t *testing.T) {
	srv, mem := newForkTestServer(t)
	parent := newTestParentSession("user_a")
	parent.Metadata.CodeAgentRuntime = types.CodeAgentRuntimeClaudeCode
	parentInteractions := seedParentWithInteractions(t, mem, parent, 3)
	user := &types.User{ID: parent.Owner, Type: types.OwnerTypeUser}
	ctx := context.Background()

	child, httpErr := srv.forkSessionFromParent(ctx, user, parent, types.CodeAgentRuntimeQwenCode, "")
	require.Nil(t, httpErr)
	require.NotNil(t, child)
	assert.NotEqual(t, parent.ID, child.ID, "child must get a fresh session id")

	// Lineage metadata on child.
	assert.Equal(t, parent.ID, child.Metadata.ParentSessionID)
	assert.WithinDuration(t, time.Now(), child.Metadata.ForkedAt, 5*time.Second)
	assert.Equal(t, parentInteractions[len(parentInteractions)-1].ID, child.Metadata.ForkedAtInteractionID)
	assert.Equal(t, types.CodeAgentRuntimeQwenCode, child.Metadata.CodeAgentRuntime)
	assert.Equal(t, "qwen", child.Metadata.ZedAgentName)
	// Inherited bits.
	assert.Equal(t, parent.Metadata.SystemPrompt, child.Metadata.SystemPrompt)
	assert.Equal(t, parent.Metadata.SpecTaskID, child.Metadata.SpecTaskID)
	assert.Equal(t, parent.ProjectID, child.ProjectID)
	assert.Equal(t, parent.Owner, child.Owner)
	// Same parent app inherited when no override.
	assert.Equal(t, parent.ParentApp, child.ParentApp)

	// fork_seed interaction on child.
	childInteractions, _, err := mem.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    child.ID,
		GenerationID: child.GenerationID,
		PerPage:      100,
	})
	require.NoError(t, err)
	require.Len(t, childInteractions, 1)
	seed := childInteractions[0]
	assert.Equal(t, types.InteractionTriggerForkSeed, seed.Trigger)
	assert.Equal(t, types.InteractionStateComplete, seed.State)
	assert.Contains(t, seed.PromptMessage, parent.ID)
	assert.Contains(t, seed.PromptMessage, "turn 3")
	// Transcript contains all three parent turns.
	assert.Contains(t, seed.ResponseMessage, "user turn 0")
	assert.Contains(t, seed.ResponseMessage, "agent reply 2")

	// Parent paused with link to child.
	freshParent, err := mem.GetSession(ctx, parent.ID)
	require.NoError(t, err)
	assert.True(t, freshParent.Metadata.Paused)
	assert.Equal(t, "forked_to:"+child.ID, freshParent.Metadata.PausedReason)
	assert.WithinDuration(t, time.Now(), freshParent.Metadata.PausedAt, 5*time.Second)
}

func TestForkSessionFromParent_OverridesParentAppWhenTargetGiven(t *testing.T) {
	srv, mem := newForkTestServer(t)
	parent := newTestParentSession("user_a")
	seedParentWithInteractions(t, mem, parent, 1)
	user := &types.User{ID: parent.Owner, Type: types.OwnerTypeUser}

	child, httpErr := srv.forkSessionFromParent(context.Background(), user, parent, types.CodeAgentRuntimeQwenCode, "app_new")
	require.Nil(t, httpErr)
	assert.Equal(t, "app_new", child.ParentApp)
}

func TestForkSessionFromParent_EmptyParentTranscript(t *testing.T) {
	srv, mem := newForkTestServer(t)
	parent := newTestParentSession("user_a")
	// No interactions at all (e.g. user forks the moment the session opens).
	_, err := mem.CreateSession(context.Background(), *parent)
	require.NoError(t, err)
	user := &types.User{ID: parent.Owner, Type: types.OwnerTypeUser}

	child, httpErr := srv.forkSessionFromParent(context.Background(), user, parent, types.CodeAgentRuntimeQwenCode, "")
	require.Nil(t, httpErr)
	assert.Equal(t, "", child.Metadata.ForkedAtInteractionID)

	childInteractions, _, err := mem.ListInteractions(context.Background(), &types.ListInteractionsQuery{
		SessionID:    child.ID,
		GenerationID: child.GenerationID,
		PerPage:      100,
	})
	require.NoError(t, err)
	require.Len(t, childInteractions, 1)
	seed := childInteractions[0]
	assert.Equal(t, "", seed.ResponseMessage, "transcript must be empty when parent had no completed turns")
	assert.Contains(t, seed.PromptMessage, "turn 0")
}

func TestForkSessionFromParent_SkipsForkSeedFromParentTranscript(t *testing.T) {
	srv, mem := newForkTestServer(t)
	// Simulate B = fork of A. Now C = fork of B. Build B with one fork_seed
	// from A plus one normal completed interaction; the seed for C should
	// contain B's real interaction but NOT the verbatim fork_seed marker.
	b := newTestParentSession("user_a")
	b.ID = "ses_B"
	_, err := mem.CreateSession(context.Background(), *b)
	require.NoError(t, err)
	_, err = mem.CreateInteraction(context.Background(), &types.Interaction{
		SessionID:       b.ID,
		GenerationID:    b.GenerationID,
		Trigger:         types.InteractionTriggerForkSeed,
		State:           types.InteractionStateComplete,
		PromptMessage:   "Session forked from ses_A at turn 4",
		ResponseMessage: "**User:** original_question\n\n**Assistant:** original_answer",
	})
	require.NoError(t, err)
	_, err = mem.CreateInteraction(context.Background(), &types.Interaction{
		SessionID:       b.ID,
		GenerationID:    b.GenerationID,
		State:           types.InteractionStateComplete,
		PromptMessage:   "follow-up on B",
		ResponseMessage: "follow-up answer on B",
	})
	require.NoError(t, err)

	user := &types.User{ID: b.Owner, Type: types.OwnerTypeUser}
	c, httpErr := srv.forkSessionFromParent(context.Background(), user, b, types.CodeAgentRuntimeQwenCode, "")
	require.Nil(t, httpErr)

	childInteractions, _, err := mem.ListInteractions(context.Background(), &types.ListInteractionsQuery{
		SessionID: c.ID, GenerationID: c.GenerationID, PerPage: 100,
	})
	require.NoError(t, err)
	require.Len(t, childInteractions, 1)
	seedForC := childInteractions[0]

	// B's fork_seed is excluded from the C transcript (serializeTranscript
	// strips fork_seed entries). B's actual user turn IS included.
	assert.NotContains(t, seedForC.ResponseMessage, "original_question")
	assert.NotContains(t, seedForC.ResponseMessage, "Session forked from ses_A")
	assert.Contains(t, seedForC.ResponseMessage, "follow-up on B")
	assert.Contains(t, seedForC.ResponseMessage, "follow-up answer on B")
}
