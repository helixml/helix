package services

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

// pollCIStatusForPR fetches the latest CI verdict for the given PR's head
// SHA and updates the in-memory RepoPR. When the cached HeadSHA differs
// from the new one, the cached CIStatus is reset (a new push happened, so
// the previous verdict is stale and we want a clean transition the next
// time CI completes). When the new state differs from the previous, a
// transition is fired: chat message to the agent + attention event to
// the human.
//
// Returns true if the RepoPR was mutated and the caller should persist
// the parent task. Errors are logged and swallowed — never abort the
// caller's loop on a single PR's CI lookup failure.
func (o *SpecTaskOrchestrator) pollCIStatusForPR(
	ctx context.Context,
	task *types.SpecTask,
	repoPR *types.RepoPR,
	pr *types.PullRequest,
) bool {
	headSHA := pr.HeadSHA
	if headSHA == "" {
		// Provider didn't surface a head SHA — nothing to query against.
		return false
	}

	prevState := repoPR.CIStatus
	prevHead := repoPR.CIHeadSHA
	mutated := false

	// New commit → reset prevState so we don't suppress a transition
	// notification when the next commit's CI lands. Persist the SHA
	// reset even if we fail to fetch the new verdict.
	if prevHead != "" && prevHead != headSHA {
		repoPR.CIStatus = ""
		repoPR.CIHeadSHA = headSHA
		repoPR.CIUpdatedAt = time.Now()
		prevState = ""
		mutated = true
	}

	status, err := o.gitService.GetCIStatus(ctx, repoPR.RepositoryID, repoPR.PRID, headSHA)
	if err != nil {
		log.Debug().
			Err(err).
			Str("task_id", task.ID).
			Str("repo_id", repoPR.RepositoryID).
			Str("pr_id", repoPR.PRID).
			Msg("Failed to fetch CI status, leaving cached value")
		return mutated
	}
	if status == nil {
		return mutated
	}

	if status.State != prevState || repoPR.CIURL != status.URL || repoPR.CIHeadSHA != headSHA {
		repoPR.CIStatus = status.State
		repoPR.CIURL = status.URL
		repoPR.CIHeadSHA = headSHA
		repoPR.CIUpdatedAt = time.Now()
		mutated = true
	}

	o.handleCIStatusTransition(ctx, task, repoPR, prevState, status.State)
	return mutated
}

// handleCIStatusTransition fires notifications when the CI verdict
// transitions from "running" to a terminal state. We only notify on
// running→passed and running→failed; first-observation (prev == "") is
// silent because we don't know what happened before we started watching.
func (o *SpecTaskOrchestrator) handleCIStatusTransition(
	ctx context.Context,
	task *types.SpecTask,
	repoPR *types.RepoPR,
	prevState, newState string,
) {
	if prevState != CIStatusRunning {
		return
	}
	if newState != CIStatusPassed && newState != CIStatusFailed {
		return
	}

	prRef := fmt.Sprintf("PR #%d (%s)", repoPR.PRNumber, repoPR.RepositoryName)
	var msg string
	var eventType types.AttentionEventType
	switch newState {
	case CIStatusPassed:
		msg = fmt.Sprintf("CI passed for %s. %s", prRef, repoPR.CIURL)
		eventType = types.AttentionEventCIPassed
	case CIStatusFailed:
		msg = fmt.Sprintf("CI failed for %s. Check the logs: %s. Please investigate and push a fix.", prRef, repoPR.CIURL)
		eventType = types.AttentionEventCIFailed
	}

	log.Info().
		Str("task_id", task.ID).
		Str("repo_id", repoPR.RepositoryID).
		Str("pr_id", repoPR.PRID).
		Str("ci_state", newState).
		Msg("CI status transition")

	if o.ciNotifier != nil {
		// Best-effort delivery; the notifier persists for offline agents.
		if err := o.ciNotifier.NotifyCIResult(ctx, task, repoPR, msg); err != nil {
			log.Warn().
				Err(err).
				Str("task_id", task.ID).
				Str("ci_state", newState).
				Msg("Failed to notify agent of CI result")
		}
	}

	if o.attentionService != nil {
		go func(t *types.SpecTask, evt types.AttentionEventType, prID, ciURL string) {
			_, err := o.attentionService.EmitEvent(
				context.Background(),
				evt,
				t,
				prID,
				map[string]interface{}{
					"pr_id":  prID,
					"ci_url": ciURL,
				},
			)
			if err != nil {
				log.Warn().Err(err).
					Str("spec_task_id", t.ID).
					Str("event_type", string(evt)).
					Msg("Failed to emit CI attention event")
			}
		}(task, eventType, repoPR.PRID, repoPR.CIURL)
	}
}
