package server

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// createAgentThreadSession creates the Helix session for a Zed thread that the
// agent opened itself (a thread Helix did not create). Ownership and linkage —
// owner, organization, project, spec task and parent — come only from the
// connection's own session, never from the agent's message, so the new session
// shows up wherever its connection's sessions do (task and project listings,
// audits) and cannot be steered into another tenant.
//
// connectionID is the id the agent's WebSocket connected with; owner is the
// session that connection belongs to (the same session unless the agent
// connected with a separate agent id).
//
// It also maps the thread to the session, registers the connection for it, and
// for spec-task connections records the work session and Zed thread.
func (apiServer *HelixAPIServer) createAgentThreadSession(ctx context.Context, connectionID string, owner *types.Session, acpThreadID, title string) (*types.Session, error) {
	now := time.Now()
	session := types.Session{
		ID:             system.GenerateSessionID(),
		Created:        now,
		Updated:        now,
		Mode:           types.SessionModeInference,
		Type:           owner.Type,
		ModelName:      owner.ModelName,
		ParentApp:      owner.ParentApp,
		ParentSession:  owner.ID,
		OrganizationID: owner.OrganizationID,
		ProjectID:      owner.ProjectID,
		Owner:          owner.Owner,
		OwnerType:      owner.OwnerType,
		Metadata: types.SessionMetadata{
			ZedThreadID:         acpThreadID,
			ExternalAgentID:     connectionID,
			AgentType:           owner.Metadata.AgentType,
			ExternalAgentConfig: owner.Metadata.ExternalAgentConfig,
			SpecTaskID:          owner.Metadata.SpecTaskID,
			ProjectID:           owner.Metadata.ProjectID,
			CodeAgentRuntime:    owner.Metadata.CodeAgentRuntime,
			CodeAgentConfig:     owner.Metadata.CodeAgentConfig,
			CodeAgentOverrides:  owner.Metadata.CodeAgentOverrides,
		},
		Name: title,
	}
	created, err := apiServer.Controller.Options.Store.CreateSession(ctx, session)
	if err != nil {
		return nil, fmt.Errorf("failed to create session for thread %s: %w", acpThreadID, err)
	}
	session = *created

	if specTaskID := session.Metadata.SpecTaskID; specTaskID != "" {
		apiServer.recordSpecTaskThread(ctx, owner.ID, &session, acpThreadID, title)
	}

	apiServer.contextMappingsMutex.Lock()
	apiServer.contextMappings[routeKey(connectionID, acpThreadID)] = session.ID
	apiServer.contextMappingsMutex.Unlock()

	// Child sessions share their parent's WebSocket; register it under the
	// child's ID so sendCommandToExternalAgent can route commands to it.
	if wsConn, exists := apiServer.externalAgentWSManager.getConnection(connectionID); exists && wsConn != nil {
		apiServer.externalAgentWSManager.registerConnection(session.ID, wsConn)
	}

	log.Info().
		Str("acp_thread_id", acpThreadID).
		Str("helix_session_id", session.ID).
		Str("connection_id", connectionID).
		Str("owner_session_id", owner.ID).
		Str("spec_task_id", session.Metadata.SpecTaskID).
		Str("project_id", session.ProjectID).
		Msg("✅ [HELIX] Created session for agent-created Zed thread")
	return &session, nil
}

// connectionOwnerSession loads the session an agent connection belongs to: the
// connection id itself for a sandbox that connected with its session id, or the
// session it was mapped to when it connected with a separate agent id.
func (apiServer *HelixAPIServer) connectionOwnerSession(ctx context.Context, connectionID string) (*types.Session, error) {
	ownerID := connectionID
	apiServer.contextMappingsMutex.RLock()
	if mapped, ok := apiServer.externalAgentSessionMapping[connectionID]; ok && mapped != "" {
		ownerID = mapped
	}
	apiServer.contextMappingsMutex.RUnlock()
	return apiServer.Controller.Options.Store.GetSession(ctx, ownerID)
}

// recordSpecTaskThread wires a new thread's session into the spec task's
// multi-session model (work session + Zed thread) so it appears in the task's
// session list and lifecycle. Failures leave the session itself in place.
func (apiServer *HelixAPIServer) recordSpecTaskThread(ctx context.Context, ownerSessionID string, session *types.Session, acpThreadID, title string) {
	st := apiServer.Controller.Options.Store
	specTaskID := session.Metadata.SpecTaskID

	phase := types.SpecTaskPhaseImplementation
	if existing, err := st.GetSpecTaskWorkSessionByHelixSession(ctx, ownerSessionID); err == nil && existing != nil {
		phase = existing.Phase
	}

	workSession := &types.SpecTaskWorkSession{
		SpecTaskID:     specTaskID,
		HelixSessionID: session.ID,
		Name:           title,
		Phase:          phase,
		Status:         types.SpecTaskWorkSessionStatusActive,
	}
	if err := st.CreateSpecTaskWorkSession(ctx, workSession); err != nil {
		log.Warn().Err(err).
			Str("spec_task_id", specTaskID).
			Str("helix_session_id", session.ID).
			Msg("Failed to create work session for agent-created thread (session still created)")
		return
	}
	now := time.Now()
	zedThread := &types.SpecTaskZedThread{
		WorkSessionID:  workSession.ID,
		SpecTaskID:     specTaskID,
		ZedThreadID:    acpThreadID,
		Status:         types.SpecTaskZedStatusActive,
		LastActivityAt: &now,
	}
	if err := st.CreateSpecTaskZedThread(ctx, zedThread); err != nil {
		log.Warn().Err(err).
			Str("spec_task_id", specTaskID).
			Str("helix_session_id", session.ID).
			Msg("Failed to create Zed thread record (work session still created)")
	}
}
