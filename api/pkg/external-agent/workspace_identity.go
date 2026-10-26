package external_agent

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// workspaceLostThread reports whether the session's agent thread died with its
// workspace. Every harness keeps its session store inside the workspace, so a
// workspace other than the one the thread was created in cannot load it.
//
// A session with no recorded workspace id predates identity stamping; for it,
// only a workspace hydra created in this very start counts as new.
func workspaceLostThread(metadata types.SessionMetadata, workspaceID string, created bool) bool {
	if metadata.ZedThreadID == "" || workspaceID == "" {
		return false
	}
	if metadata.WorkspaceID != "" {
		return metadata.WorkspaceID != workspaceID
	}
	return created
}

// reconcileWorkspaceIdentity records the workspace the session now runs in and,
// when that workspace no longer holds the session's thread, clears the thread
// id and marks the session for a transcript reseed (ThreadReseedFrom). This is
// the deterministic counterpart to error-based recovery: it needs no knowledge
// of how each harness reports a missing thread.
func (h *HydraExecutor) reconcileWorkspaceIdentity(ctx context.Context, sessionID string, resp *hydra.DevContainerResponse) error {
	if resp == nil || resp.WorkspaceID == "" {
		return nil
	}
	session, err := h.store.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load session %s to record workspace identity: %w", sessionID, err)
	}
	metadata := session.Metadata
	if metadata.WorkspaceID == resp.WorkspaceID && !resp.WorkspaceCreated {
		return nil
	}
	if workspaceLostThread(metadata, resp.WorkspaceID, resp.WorkspaceCreated) {
		log.Info().
			Str("session_id", sessionID).
			Str("old_thread_id", metadata.ZedThreadID).
			Str("previous_workspace_id", metadata.WorkspaceID).
			Str("workspace_id", resp.WorkspaceID).
			Bool("workspace_created", resp.WorkspaceCreated).
			Msg("Workspace was recreated; the agent thread is gone, next message opens a reseeded thread")
		metadata.ThreadReseedFrom = metadata.ZedThreadID
		metadata.ZedThreadID = ""
	}
	metadata.WorkspaceID = resp.WorkspaceID
	if err := h.store.UpdateSessionMetadata(ctx, sessionID, metadata); err != nil {
		return fmt.Errorf("record workspace identity for session %s: %w", sessionID, err)
	}
	return nil
}
