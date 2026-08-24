// Package helix is the production Spawner runtime: each AI Worker
// activation drives a chat session against a co-located Helix server.
//
// Per-Worker state — the Helix project ID, the auto-provisioned Agent
// App ID, the project's primary git repo ID, and the live chat session
// pointer — lives in the NodeRuntimeState sidecar store under the
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

// Backend is the label used in NodeRuntimeState to namespace this
// runtime's per-Worker keys.
const Backend = "helix"

const (
	Runtime   = "zed_agent"
	AgentType = "zed_external"
)

// WorkerState holds the per-Worker pointers the Helix runtime needs.
type WorkerState struct {
	ProjectID    string
	AgentID      string
	RepoID       string
	SessionID    string
	HiringUserID string
	// RestartRequiredContainer is the Docker container id of the Worker's
	// sandbox at the moment a restart-sensitive config change was saved.
	// Empty when nothing is pending or the sandbox was down at save time.
	RestartRequiredContainer string
}

const (
	keyProjectID    = "project_id"
	keyAgentID      = "agent_app_id"
	keyRepoID       = "repo_id"
	keySessionID    = "session_id"
	keyHiringUserID = "hiring_user_id"
	// keyRestartContainer stores the sandbox container id that a saved
	// config change made stale. Docker never reuses a container id, so
	// comparing it to the session's live ContainerID is what makes the
	// flag self-clear on every container recreate — there is deliberately
	// no code anywhere that clears this key.
	keyRestartContainer = "restart_required_container"
)

// LoadState returns the Helix-backend state for a Worker.
func LoadState(ctx context.Context, st *store.Store, orgID string, workerID orgchart.NodeID) (WorkerState, error) {
	if st == nil || st.NodeRuntimeState == nil {
		return WorkerState{}, errors.New("helix state: store is nil")
	}
	kv, err := st.NodeRuntimeState.Get(ctx, orgID, workerID, Backend)
	if err != nil {
		return WorkerState{}, fmt.Errorf("helix state: get %s/%s: %w", orgID, workerID, err)
	}
	return WorkerState{
		ProjectID:            kv[keyProjectID],
		AgentID:              kv[keyAgentID],
		RepoID:               kv[keyRepoID],
		SessionID:            kv[keySessionID],
		HiringUserID:         kv[keyHiringUserID],
		RestartRequiredContainer: kv[keyRestartContainer],
	}, nil
}

// SaveHiringUser persists the user identifier that called hire_worker.
func SaveHiringUser(ctx context.Context, st *store.Store, orgID string, workerID orgchart.NodeID, userID string) error {
	if userID == "" {
		return nil
	}
	if st == nil || st.NodeRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.NodeRuntimeState.Set(ctx, orgID, workerID, Backend, keyHiringUserID, userID)
}

// SaveProject persists the per-Worker project triple.
func SaveProject(ctx context.Context, st *store.Store, orgID string, workerID orgchart.NodeID, projectID, agentID, repoID string) error {
	if st == nil || st.NodeRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.NodeRuntimeState.SetMany(ctx, orgID, workerID, Backend, map[string]string{
		keyProjectID: projectID,
		keyAgentID:   agentID,
		keyRepoID:    repoID,
	})
}

// SaveSession persists the live Helix chat session ID.
func SaveSession(ctx context.Context, st *store.Store, orgID string, workerID orgchart.NodeID, sessionID string) error {
	if st == nil || st.NodeRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.NodeRuntimeState.Set(ctx, orgID, workerID, Backend, keySessionID, sessionID)
}

// ClearProject nulls the project triple AND the session pointer.
func ClearProject(ctx context.Context, st *store.Store, orgID string, workerID orgchart.NodeID) error {
	if st == nil || st.NodeRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.NodeRuntimeState.SetMany(ctx, orgID, workerID, Backend, map[string]string{
		keyProjectID: "",
		keyAgentID:   "",
		keyRepoID:    "",
		keySessionID: "",
	})
}

// SaveRestartRequiredContainer records which sandbox container was live
// when a restart-sensitive config change was saved. Writing "" (no
// session, or the sandbox is down) is meaningful: it is the no-banner
// case, so this deliberately does not skip empty values the way
// SaveHiringUser does.
func SaveRestartRequiredContainer(ctx context.Context, st *store.Store, orgID string, workerID orgchart.NodeID, containerID string) error {
	if st == nil || st.NodeRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.NodeRuntimeState.Set(ctx, orgID, workerID, Backend, keyRestartContainer, containerID)
}
