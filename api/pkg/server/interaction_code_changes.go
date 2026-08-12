package server

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

var checkpointRefSegmentRE = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// codeChangesCaptureTimeout bounds every desktop round-trip made for
// checkpoint capture. Both capture paths run on the external-agent
// WebSocket read goroutine, which also dispatches pongs and carries a
// 60s read deadline: an unbounded capture stalls the session's whole
// sync stream and, past 60s, tears the connection down. A capture that
// cannot finish inside this budget is recorded as a failed receipt
// instead of being waited on.
const codeChangesCaptureTimeout = 15 * time.Second

func checkpointRef(sessionID, interactionID, phase string) string {
	sessionSegment := safeCheckpointRefSegment(sessionID)
	interactionSegment := safeCheckpointRefSegment(interactionID)
	return fmt.Sprintf("refs/helix/checkpoints/%s/%s/%s", sessionSegment, interactionSegment, phase)
}

func safeCheckpointRefSegment(value string) string {
	value = checkpointRefSegmentRE.ReplaceAllString(value, "-")
	for strings.Contains(value, "..") {
		value = strings.ReplaceAll(value, "..", "-")
	}
	value = strings.Trim(value, ".-")
	if value == "" {
		return "unknown"
	}
	return value
}

func (apiServer *HelixAPIServer) captureInteractionBeforeCheckpoint(sessionID string, command types.ExternalAgentCommand) {
	if command.Type != "chat_message" {
		return
	}
	trackCodeChanges, _ := command.Data["track_code_changes"].(bool)
	if !trackCodeChanges {
		return
	}
	interactionID, _ := command.Data["interaction_id"].(string)
	if interactionID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), codeChangesCaptureTimeout)
	defer cancel()
	interaction, err := apiServer.Store.GetInteraction(ctx, interactionID)
	if err != nil || interaction == nil || interaction.SessionID != sessionID {
		return
	}
	if interaction.CodeChanges != nil && interaction.CodeChanges.BeforeRef != "" {
		return
	}

	beforeRef := checkpointRef(sessionID, interactionID, "before")
	var captured types.WorkspaceCheckpointCaptureResponse
	err = apiServer.callSessionDesktopJSON(ctx, sessionID, http.MethodPost, "/workspace/checkpoints/capture", types.WorkspaceCheckpointCaptureRequest{
		Ref: beforeRef,
	}, &captured)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Str("interaction_id", interactionID).
			Msg("Failed to capture interaction before checkpoint")
		interaction.CodeChanges = &types.InteractionCodeChanges{
			Status: types.CodeChangesStatusMissing, Files: []types.InteractionCodeChangeFile{}, Error: err.Error(),
		}
	} else {
		interaction.CodeChanges = &types.InteractionCodeChanges{
			Status: types.CodeChangesStatusCapturing, Workspace: captured.Workspace,
			BeforeRef: beforeRef, Files: []types.InteractionCodeChangeFile{}, CapturedAt: captured.Captured,
		}
	}
	interaction.Updated = time.Now()
	if _, updateErr := apiServer.Store.UpdateInteraction(ctx, interaction); updateErr != nil {
		log.Warn().Err(updateErr).Str("interaction_id", interactionID).
			Msg("Failed to persist interaction before checkpoint")
	}
}

func (apiServer *HelixAPIServer) finalizeInteractionCodeChanges(ctx context.Context, sessionID string, interaction *types.Interaction) {
	if interaction == nil {
		return
	}
	// Runs inline on the sync read loop so the receipt is durable before the
	// terminal interaction update publishes. The budget is what keeps that
	// safe — see codeChangesCaptureTimeout.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		bounded, cancel := context.WithTimeout(ctx, codeChangesCaptureTimeout)
		defer cancel()
		ctx = bounded
	}
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil || session == nil || session.Metadata.SpecTaskID == "" {
		return
	}
	if interaction.CodeChanges != nil && interaction.CodeChanges.Status == types.CodeChangesStatusReady {
		return
	}

	// The before checkpoint is persisted synchronously before the command is sent,
	// while the websocket completion path may still hold an older in-memory copy
	// of the interaction. Reload that metadata before deciding it is missing.
	if interaction.CodeChanges == nil || interaction.CodeChanges.BeforeRef == "" {
		persisted, getErr := apiServer.Store.GetInteraction(ctx, interaction.ID)
		if getErr == nil && persisted != nil && persisted.CodeChanges != nil {
			recovered := *persisted.CodeChanges
			recovered.Files = append([]types.InteractionCodeChangeFile(nil), persisted.CodeChanges.Files...)
			interaction.CodeChanges = &recovered
		}
	}
	if interaction.CodeChanges != nil && interaction.CodeChanges.Status == types.CodeChangesStatusReady {
		return
	}
	if interaction.CodeChanges == nil || interaction.CodeChanges.BeforeRef == "" {
		errorMessage := "before checkpoint unavailable"
		if interaction.CodeChanges != nil && interaction.CodeChanges.Error != "" {
			errorMessage = interaction.CodeChanges.Error
		}
		interaction.CodeChanges = &types.InteractionCodeChanges{
			Status: types.CodeChangesStatusMissing, Files: []types.InteractionCodeChangeFile{},
			Error: errorMessage,
		}
		return
	}

	afterRef := checkpointRef(sessionID, interaction.ID, "after")
	var captured types.WorkspaceCheckpointCaptureResponse
	if err := apiServer.callSessionDesktopJSON(ctx, sessionID, http.MethodPost, "/workspace/checkpoints/capture", types.WorkspaceCheckpointCaptureRequest{
		Workspace: interaction.CodeChanges.Workspace, Ref: afterRef,
	}, &captured); err != nil {
		interaction.CodeChanges.Status = types.CodeChangesStatusError
		interaction.CodeChanges.Error = err.Error()
		return
	}

	var diff types.WorkspaceCheckpointDiffResponse
	if err := apiServer.callSessionDesktopJSON(ctx, sessionID, http.MethodPost, "/workspace/checkpoints/diff", types.WorkspaceCheckpointDiffRequest{
		Workspace: interaction.CodeChanges.Workspace,
		BeforeRef: interaction.CodeChanges.BeforeRef,
		AfterRef:  afterRef,
	}, &diff); err != nil {
		interaction.CodeChanges.Status = types.CodeChangesStatusError
		interaction.CodeChanges.AfterRef = afterRef
		interaction.CodeChanges.Error = err.Error()
		return
	}

	interaction.CodeChanges = &types.InteractionCodeChanges{
		Status: types.CodeChangesStatusReady, Workspace: captured.Workspace,
		BeforeRef: diff.BeforeRef, AfterRef: diff.AfterRef, PatchHash: diff.PatchHash,
		Files: diff.Files, TotalAdditions: diff.TotalAdditions, TotalDeletions: diff.TotalDeletions,
		CapturedAt: captured.Captured,
	}
}
