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

	// Child now owns three inherited copies of the parent's completed
	// turns plus one fork_seed divider — four interactions total. The
	// inherited rows make the fork a self-contained snapshot so a
	// future fork-of-fork preserves full ancestry.
	childInteractions, _, err := mem.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    child.ID,
		GenerationID: child.GenerationID,
		PerPage:      100,
	})
	require.NoError(t, err)
	require.Len(t, childInteractions, 4, "expected 3 inherited rows + 1 fork_seed")
	// fork_seed must be last (after the inherited rows) so it visually
	// divides "history before fork" from "child's own future turns".
	seed := childInteractions[len(childInteractions)-1]
	assert.Equal(t, types.InteractionTriggerForkSeed, seed.Trigger)
	assert.Equal(t, types.InteractionStateComplete, seed.State)
	assert.Contains(t, seed.PromptMessage, parent.ID)
	assert.Contains(t, seed.PromptMessage, "turn 3")
	// Transcript blob still contains all three parent turns (used by
	// maybePrependTranscript to seed the agent on first message).
	assert.Contains(t, seed.ResponseMessage, "user turn 0")
	assert.Contains(t, seed.ResponseMessage, "agent reply 2")
	// The first three are inherited copies of the parent's turns.
	for i := 0; i < 3; i++ {
		assert.Equal(t, types.InteractionTriggerForkInherited, childInteractions[i].Trigger,
			"interaction %d should be marked as inherited from the parent", i)
		assert.NotEqual(t, parentInteractions[i].ID, childInteractions[i].ID,
			"inherited rows must get fresh IDs, not reuse the parent's")
		assert.Equal(t, child.ID, childInteractions[i].SessionID,
			"inherited rows must be owned by the child session, not the parent")
	}

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
	require.Len(t, childInteractions, 1, "parent had no interactions to inherit, only fork_seed exists")
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
	// B had one fake-fork_seed + one real interaction. We inherit the
	// real one (fork_seed is stripped) and add C's own fork_seed.
	require.Len(t, childInteractions, 2)
	seedForC := childInteractions[len(childInteractions)-1]
	assert.Equal(t, types.InteractionTriggerForkSeed, seedForC.Trigger)

	// B's fork_seed is excluded from the C transcript (serializeTranscript
	// strips fork_seed entries). B's actual user turn IS included.
	assert.NotContains(t, seedForC.ResponseMessage, "original_question")
	assert.NotContains(t, seedForC.ResponseMessage, "Session forked from ses_A")
	assert.Contains(t, seedForC.ResponseMessage, "follow-up on B")
	assert.Contains(t, seedForC.ResponseMessage, "follow-up answer on B")
}

// TestForkSessionFromParent_ChainDepth2PreservesFullAncestry covers
// the user-reported concern: a fork of a fork must contain the
// original ancestor's turns, not just the immediate parent's own. The
// "copy interactions at fork time" design makes this fall out
// naturally — B inherits A's turns; C inherits B's full interaction
// list (which already includes A's). No chain-walking required.
func TestForkSessionFromParent_ChainDepth2PreservesFullAncestry(t *testing.T) {
	srv, mem := newForkTestServer(t)
	ctx := context.Background()

	// A: original session, two completed turns.
	a := newTestParentSession("user_a")
	a.ID = "ses_A"
	a.Metadata.CodeAgentRuntime = types.CodeAgentRuntimeClaudeCode
	seedParentWithInteractions(t, mem, a, 2) // creates "user turn 0/1", "agent reply 0/1"
	user := &types.User{ID: a.Owner, Type: types.OwnerTypeUser}

	// Fork A → B.
	b, httpErr := srv.forkSessionFromParent(ctx, user, a, types.CodeAgentRuntimeQwenCode, "")
	require.Nil(t, httpErr)

	// Add a turn directly on B so chain depth 2 actually shows
	// continuation beyond the inherited bits.
	_, err := mem.CreateInteraction(ctx, &types.Interaction{
		SessionID:       b.ID,
		GenerationID:    b.GenerationID,
		State:           types.InteractionStateComplete,
		PromptMessage:   "B-only follow up",
		ResponseMessage: "B-only follow-up answer",
	})
	require.NoError(t, err)

	// B must NOT be paused before we re-fork it.
	freshB, err := mem.GetSession(ctx, b.ID)
	require.NoError(t, err)
	freshB.Metadata.Paused = false
	freshB.Metadata.PausedReason = ""
	_, err = mem.UpdateSession(ctx, *freshB)
	require.NoError(t, err)

	// Fork B → C.
	c, httpErr := srv.forkSessionFromParent(ctx, user, freshB, types.CodeAgentRuntimeGeminiCLI, "")
	require.Nil(t, httpErr)

	cInteractions, _, err := mem.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: c.ID, GenerationID: c.GenerationID, PerPage: 100,
	})
	require.NoError(t, err)

	// Expected on C (in order):
	//   2 inherited copies from A (user turn 0, user turn 1) — note that
	//     B inherited these as fork_inherited rows, and C inherits B's
	//     full non-fork_seed history → these come along
	//   1 inherited copy of B's own follow-up
	//   1 fork_seed marking the C fork
	require.Len(t, cInteractions, 4, "C must inherit A's 2 turns + B's 1 turn + own fork_seed")

	prompts := make([]string, 0, len(cInteractions))
	for _, in := range cInteractions {
		prompts = append(prompts, in.PromptMessage)
	}
	assert.Contains(t, prompts[0], "user turn 0", "A's first turn must be present in C")
	assert.Contains(t, prompts[1], "user turn 1", "A's second turn must be present in C")
	assert.Contains(t, prompts[2], "B-only follow up", "B's own turn must be present in C")
	assert.Equal(t, types.InteractionTriggerForkSeed, cInteractions[3].Trigger)

	// All the historical rows are marked as inherited so the UI can
	// hide destructive actions on them.
	for i := 0; i < 3; i++ {
		assert.Equal(t, types.InteractionTriggerForkInherited, cInteractions[i].Trigger,
			"chain-inherited interactions must keep the fork_inherited marker")
	}
}
