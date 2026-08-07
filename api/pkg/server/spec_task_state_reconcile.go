package server

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// reconcileSpecTaskAfterTurn releases a spec task's latched failure state once
// its agent has demonstrably resumed work, and advances it out of backlog.
//
// A task and its session are two state machines, and only the spec-task
// orchestration routes (start-planning / start-implementation) ever cleared
// `metadata.error` or advanced status. Work can resume without touching them:
// the chat's Retry — and an ordinary chat message — go through session
// inference, which has no spec-task awareness. The task then sits at
// `backlog` with a stale error while its agent codes, and the toolbar keeps
// offering "Retry Implementation" because that label is derived from the
// latched error rather than from observable state.
//
// A completed turn is proof the failure is no longer true, so this releases
// the latch and moves the task out of backlog. Capacity accounting is not
// bypassed by doing so — the work is already running and already consuming a
// slot; leaving the task in backlog is what hides it from the ledger.
func (apiServer *HelixAPIServer) reconcileSpecTaskAfterTurn(ctx context.Context, session *types.Session) {
	apiServer.reconcileSpecTaskState(ctx, session, true)
}

// reconcileSpecTaskLaunchFailure releases only the latched launch error, leaving
// status alone.
//
// The recorded failure says the desktop could not start. An agent on the wire
// proves that is no longer true, whatever happens to the turn afterwards —
// and something must release the latch on the path where the agent connects,
// works, and the sandbox idles out without a turn ever completing. Status still
// requires evidence of finished work, so it is left to the turn path.
func (apiServer *HelixAPIServer) reconcileSpecTaskLaunchFailure(ctx context.Context, sessionID string) {
	if apiServer.Store == nil || sessionID == "" {
		return
	}
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil || session == nil {
		return
	}
	apiServer.reconcileSpecTaskState(ctx, session, false)
}

func (apiServer *HelixAPIServer) reconcileSpecTaskState(ctx context.Context, session *types.Session, advanceStatus bool) {
	if session == nil || session.Metadata.SpecTaskID == "" {
		return
	}
	store := apiServer.Store
	if store == nil {
		return
	}
	task, err := store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
	if err != nil || task == nil {
		return
	}

	changed := false
	for _, key := range []string{"error", "error_timestamp", types.TaskErrorCodeKey, types.TaskErrorProviderKey} {
		if _, ok := task.Metadata[key]; ok {
			delete(task.Metadata, key)
			changed = true
		}
	}

	// Only backlog is reconciled. Every other status is either a live phase the
	// orchestrator owns or a terminal one a finished turn must not reopen.
	if advanceStatus && task.Status == types.TaskStatusBacklog {
		task.Status = resumedSpecTaskStatus(task)
		now := time.Now()
		task.StatusUpdatedAt = &now
		changed = true
	}
	if !changed {
		return
	}

	task.UpdatedAt = time.Now()
	if err := store.UpdateSpecTask(ctx, task); err != nil {
		log.Warn().Err(err).
			Str("task_id", task.ID).
			Str("session_id", session.ID).
			Msg("Failed to reconcile spec task after completed turn")
		return
	}
	log.Info().
		Str("task_id", task.ID).
		Str("session_id", session.ID).
		Str("status", string(task.Status)).
		Msg("Reconciled spec task state after its agent resumed work")
}

// resumedSpecTaskStatus is the phase the orchestration routes would have set
// for this task, so a reconciled task lands where a normal start would put it.
func resumedSpecTaskStatus(task *types.SpecTask) types.SpecTaskStatus {
	if task.JustDoItMode {
		return types.TaskStatusImplementation
	}
	return types.TaskStatusSpecGeneration
}
