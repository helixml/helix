// Package helix is the production Spawner runtime: each AI Worker
// activation drives a chat session against a co-located Helix server.
//
// Per-Worker state — the Helix project ID, the auto-provisioned Agent
// App ID, the project's primary git repo ID, and the live chat session
// pointer — lives in the WorkerRuntimeState sidecar store under the
// "helix" backend label. The accessors in state.go give the rest of
// this package typed access without leaking key strings everywhere.
package helix

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/store"
)

// Backend is the label used in WorkerRuntimeState to namespace this
// runtime's per-Worker keys. helix-org core never reads it; it's
// here only so every helix-runtime call site spells the same string.
const Backend = "helix"

// Runtime and AgentType are the only Helix project / session shape
// helix-org uses. Every per-Worker project (owner-chat session AND
// AI-worker activation) is applied with `Runtime=zed_agent` and every
// `/sessions/chat` POST sets `agent_type=zed_external`. Two reasons
// to make these constants rather than configurable:
//
//   - `claude_code` runtime ignores the project's Provider/Model and
//     talks directly to Anthropic via its own creds — empirically
//     hangs in `state=waiting` on app.helix.ml because the in-sandbox
//     agent can't reach Anthropic. `zed_agent` routes inference back
//     through Helix and honours the configured provider/model.
//   - Mixing `claude_code` for AI workers and `zed_agent` for owner
//     chat creates two completely different sandbox shapes for the
//     "same" runtime — confusing to debug and impossible to reason
//     about. ONE shape, ALWAYS.
//
// Helix's own `helix_basic` agent_type doesn't route MCP tool calls
// in inference — only `zed_external` does — so we'd never want it
// for the chat surface or worker activations either way.
const (
	Runtime   = "zed_agent"
	AgentType = "zed_external"
)

// WorkerState holds the per-Worker pointers the Helix runtime needs.
// All five fields are empty for a Worker that hasn't been activated
// yet; the runtime's first activation materialises ProjectID +
// AgentAppID + RepoID via WorkerProject.Ensure, and SessionID is
// set when the first chat session opens.
//
// HiringUserID is the identity (typically a Helix user_id) of
// whoever called hire_worker, captured from request context. The
// Spawner forwards this to the embedding host's `BearerForUser`
// callback to mint/look up a fresh api_key at activation time —
// so each Worker runs on the hiring user's subscription, quota,
// and audit trail without persisting any token at rest. Empty in
// standalone helix-org (no HTTP auth) and any deploy without a
// per-user identity to capture; the Spawner then falls back to
// the static service api_key.
type WorkerState struct {
	ProjectID    string
	AgentAppID   string
	RepoID       string
	SessionID    string
	HiringUserID string
}

const (
	keyProjectID    = "project_id"
	keyAgentAppID   = "agent_app_id"
	keyRepoID       = "repo_id"
	keySessionID    = "session_id"
	keyHiringUserID = "hiring_user_id"
)

// LoadState returns the Helix-backend state for a Worker. Empty
// fields mean "not yet set"; never an error path.
func LoadState(ctx context.Context, st *store.Store, workerID worker.ID) (WorkerState, error) {
	if st == nil || st.WorkerRuntimeState == nil {
		return WorkerState{}, errors.New("helix state: store is nil")
	}
	kv, err := st.WorkerRuntimeState.Get(ctx, workerID, Backend)
	if err != nil {
		return WorkerState{}, fmt.Errorf("helix state: get %s: %w", workerID, err)
	}
	return WorkerState{
		ProjectID:    kv[keyProjectID],
		AgentAppID:   kv[keyAgentAppID],
		RepoID:       kv[keyRepoID],
		SessionID:    kv[keySessionID],
		HiringUserID: kv[keyHiringUserID],
	}, nil
}

// SaveHiringUser persists the user identifier that called
// hire_worker for this Worker. Empty userID is a no-op so re-hire /
// re-activation in unauthenticated contexts doesn't wipe a user
// captured by an earlier authenticated hire.
func SaveHiringUser(ctx context.Context, st *store.Store, workerID worker.ID, userID string) error {
	if userID == "" {
		return nil
	}
	if st == nil || st.WorkerRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.WorkerRuntimeState.Set(ctx, workerID, Backend, keyHiringUserID, userID)
}

// SaveProject persists the per-Worker project triple — created once
// at first activation by WorkerProject.Ensure.
func SaveProject(ctx context.Context, st *store.Store, workerID worker.ID, projectID, agentAppID, repoID string) error {
	if st == nil || st.WorkerRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.WorkerRuntimeState.SetMany(ctx, workerID, Backend, map[string]string{
		keyProjectID:  projectID,
		keyAgentAppID: agentAppID,
		keyRepoID:     repoID,
	})
}

// SaveSession persists the live Helix chat session ID. Reused across
// activations so the per-Worker desktop container stays warm.
func SaveSession(ctx context.Context, st *store.Store, workerID worker.ID, sessionID string) error {
	if st == nil || st.WorkerRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.WorkerRuntimeState.Set(ctx, workerID, Backend, keySessionID, sessionID)
}

// ClearProject nulls the project triple AND the session pointer when
// the persisted project no longer exists on the Helix side (operator
// deleted it directly). Wiping the session too prevents follow-up
// sends from attaching to a dead container.
func ClearProject(ctx context.Context, st *store.Store, workerID worker.ID) error {
	if st == nil || st.WorkerRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.WorkerRuntimeState.SetMany(ctx, workerID, Backend, map[string]string{
		keyProjectID:  "",
		keyAgentAppID: "",
		keyRepoID:     "",
		keySessionID:  "",
	})
}
