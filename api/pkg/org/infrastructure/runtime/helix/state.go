// Package helix is the production Spawner runtime: each AI Worker
// activation drives a chat session against a co-located Helix server.
//
// Per-Worker state — the Helix project ID, the auto-provisioned Agent
// App ID, the project's primary git repo ID, and the live chat session
// pointer — lives in the WorkerRuntimeState sidecar store under the
// "helix" backend label, scoped by org. The accessors in state.go give
// the rest of this package typed access without leaking key strings
// everywhere.
package helix

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

// Backend is the label used in WorkerRuntimeState to namespace this
// runtime's per-Worker keys.
const Backend = "helix"

const (
	Runtime   = "zed_agent"
	AgentType = "zed_external"
)

// WorkerState holds the per-Worker pointers the Helix runtime needs.
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

// LoadState returns the Helix-backend state for a Worker.
func LoadState(ctx context.Context, st *store.Store, orgID string, workerID orgchart.BotID) (WorkerState, error) {
	if st == nil || st.WorkerRuntimeState == nil {
		return WorkerState{}, errors.New("helix state: store is nil")
	}
	kv, err := st.WorkerRuntimeState.Get(ctx, orgID, workerID, Backend)
	if err != nil {
		return WorkerState{}, fmt.Errorf("helix state: get %s/%s: %w", orgID, workerID, err)
	}
	return WorkerState{
		ProjectID:    kv[keyProjectID],
		AgentAppID:   kv[keyAgentAppID],
		RepoID:       kv[keyRepoID],
		SessionID:    kv[keySessionID],
		HiringUserID: kv[keyHiringUserID],
	}, nil
}

// SaveHiringUser persists the user identifier that called hire_worker.
func SaveHiringUser(ctx context.Context, st *store.Store, orgID string, workerID orgchart.BotID, userID string) error {
	if userID == "" {
		return nil
	}
	if st == nil || st.WorkerRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.WorkerRuntimeState.Set(ctx, orgID, workerID, Backend, keyHiringUserID, userID)
}

// SaveProject persists the per-Worker project triple.
func SaveProject(ctx context.Context, st *store.Store, orgID string, workerID orgchart.BotID, projectID, agentAppID, repoID string) error {
	if st == nil || st.WorkerRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.WorkerRuntimeState.SetMany(ctx, orgID, workerID, Backend, map[string]string{
		keyProjectID:  projectID,
		keyAgentAppID: agentAppID,
		keyRepoID:     repoID,
	})
}

// SaveSession persists the live Helix chat session ID.
func SaveSession(ctx context.Context, st *store.Store, orgID string, workerID orgchart.BotID, sessionID string) error {
	if st == nil || st.WorkerRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.WorkerRuntimeState.Set(ctx, orgID, workerID, Backend, keySessionID, sessionID)
}

// ClearProject nulls the project triple AND the session pointer.
func ClearProject(ctx context.Context, st *store.Store, orgID string, workerID orgchart.BotID) error {
	if st == nil || st.WorkerRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.WorkerRuntimeState.SetMany(ctx, orgID, workerID, Backend, map[string]string{
		keyProjectID:  "",
		keyAgentAppID: "",
		keyRepoID:     "",
		keySessionID:  "",
	})
}
