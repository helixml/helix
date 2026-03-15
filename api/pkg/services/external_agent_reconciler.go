package services

import (
	"context"
	"fmt"
	"time"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const (
	// ReconcileInterval is how often the reconciler runs
	ReconcileInterval = 30 * time.Second
)

// ExternalAgentReconciler ensures external agent containers match their desired state
// It restarts containers that should be running but are missing (e.g., after Wolf crash)
type ExternalAgentReconciler struct {
	store           store.Store
	executor        external_agent.Executor
	specTaskService *SpecDrivenTaskService
}

// NewExternalAgentReconciler creates a new external agent reconciler
func NewExternalAgentReconciler(
	store store.Store,
	executor external_agent.Executor,
	specTaskService *SpecDrivenTaskService,
) *ExternalAgentReconciler {
	return &ExternalAgentReconciler{
		store:           store,
		executor:        executor,
		specTaskService: specTaskService,
	}
}

// Start begins the reconciliation loop
func (r *ExternalAgentReconciler) Start(ctx context.Context) {
	log.Info().Msg("Starting external agent reconciler")

	// Clean up sessions that were stuck mid-startup when the API crashed.
	// This must run before the first reconcile so the reconciler doesn't
	// try to resume a session with stale "starting" status.
	r.cleanupStaleStartingSessions(ctx)

	ticker := time.NewTicker(ReconcileInterval)
	defer ticker.Stop()

	// Run once immediately on startup
	r.reconcile(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping external agent reconciler")
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

// reconcile performs a single reconciliation pass
func (r *ExternalAgentReconciler) reconcile(ctx context.Context) {
	// Get all sessions with DesiredState = "running"
	sessions, err := r.store.ListSessionsWithDesiredState(ctx, types.DesiredStateRunning)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list sessions with desired state running")
		return
	}

	if len(sessions) == 0 {
		log.Debug().Msg("Reconcile: no sessions need running containers")
		return
	}

	log.Info().Int("count", len(sessions)).Msg("Reconciling external agent sessions")

	for _, session := range sessions {
		if err := r.reconcileSession(ctx, session); err != nil {
			log.Error().
				Err(err).
				Str("session_id", session.ID).
				Msg("Failed to reconcile session")
		}
	}
}

// cleanupStaleStartingSessions finds sessions that were left in "starting" state
// (e.g. because the API crashed during golden cache copy) and resets them so the
// normal reconciler can restart them cleanly.
//
// When the API crashes mid-CreateDevContainer:
//   - The session has external_agent_status="starting" and a stale status_message
//     like "Unpacking build cache (52.2/58.7 GB)"
//   - The container never launched (no running container on sandbox)
//   - The golden cache copy on disk may be incomplete (handled by Hydra-side
//     completion marker check in buildMounts)
//
// We clear the stale status so the frontend doesn't show a frozen progress bar,
// and the reconciler's normal "container missing" path will restart the session.
func (r *ExternalAgentReconciler) cleanupStaleStartingSessions(ctx context.Context) {
	sessions, err := r.store.ListSessionsWithDesiredState(ctx, types.DesiredStateRunning)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list sessions for stale startup cleanup")
		return
	}

	cleaned := 0
	for _, session := range sessions {
		if session.Metadata.ExternalAgentStatus != "starting" {
			continue
		}

		// Session was mid-startup when the API died. Check if the container
		// actually exists (unlikely but possible if only the goroutine died).
		if r.executor.HasRunningContainer(ctx, session.ID) {
			continue
		}

		log.Warn().
			Str("session_id", session.ID).
			Str("spec_task_id", session.Metadata.SpecTaskID).
			Str("stale_status_message", session.Metadata.StatusMessage).
			Msg("Clearing stale 'starting' session state from interrupted startup")

		// Clear the frozen status message and reset agent status so the
		// frontend stops showing the stale progress bar.
		session.Metadata.StatusMessage = ""
		session.Metadata.ExternalAgentStatus = ""
		if _, err := r.store.UpdateSession(ctx, *session); err != nil {
			log.Error().Err(err).
				Str("session_id", session.ID).
				Msg("Failed to clear stale starting session state")
		} else {
			cleaned++
		}
	}

	if cleaned > 0 {
		log.Info().
			Int("cleaned", cleaned).
			Msg("Cleared stale 'starting' sessions from previous API crash")
	}
}

// reconcileSession ensures a single session's container matches desired state
func (r *ExternalAgentReconciler) reconcileSession(ctx context.Context, session *types.Session) error {
	// Check if Wolf has a running container for this session
	hasContainer := r.executor.HasRunningContainer(ctx, session.ID)

	if hasContainer {
		// Container exists, nothing to do
		log.Debug().
			Str("session_id", session.ID).
			Msg("Container already running, nothing to reconcile")
		return nil
	}

	// Container missing but should be running - restart it
	log.Info().
		Str("session_id", session.ID).
		Str("spec_task_id", session.Metadata.SpecTaskID).
		Msg("Container missing, restarting session")

	// Get the spec task to determine restart method
	if session.Metadata.SpecTaskID == "" {
		log.Warn().
			Str("session_id", session.ID).
			Msg("Session has no SpecTaskID, cannot restart")
		return nil
	}

	task, err := r.store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
	if err != nil {
		return fmt.Errorf("failed to get spec task: %w", err)
	}

	// Check if task is still in a running state
	if task.Archived {
		log.Info().
			Str("session_id", session.ID).
			Str("spec_task_id", task.ID).
			Msg("Task is archived, clearing desired state instead of restarting")

		// Clear desired state since task is done
		session.Metadata.DesiredState = types.DesiredStateStopped
		_, err := r.store.UpdateSession(ctx, *session)
		return err
	}

	// Use the existing resume session logic
	// This calls the Wolf executor to start a new container
	err = r.specTaskService.ResumeSession(ctx, task, session)
	if err != nil {
		return fmt.Errorf("failed to resume session: %w", err)
	}

	log.Info().
		Str("session_id", session.ID).
		Str("spec_task_id", task.ID).
		Msg("Successfully restarted session container")

	return nil
}
