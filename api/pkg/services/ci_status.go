package services

import "strings"

// CI status canonical values stored on RepoPR.CIStatus.
const (
	CIStatusRunning = "running"
	CIStatusPassed  = "passed"
	CIStatusFailed  = "failed"
	CIStatusNone    = "none"
)

// NormalizeCIStatus collapses provider-specific CI verdicts to one of the
// canonical CI status values. An unknown raw value is treated as failed
// rather than silently ignored — surfacing surprises beats hiding them.
//
// provider is one of "github", "gitlab", "azure_devops", "bitbucket". An
// empty raw string returns CIStatusNone, regardless of provider.
func NormalizeCIStatus(provider, raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return CIStatusNone
	}

	switch provider {
	case "github":
		// Combined Status: success | pending | failure | error
		// Check Run conclusion: success | failure | neutral | cancelled |
		// timed_out | action_required | stale | skipped (or status when
		// queued/in_progress).
		switch raw {
		case "success", "neutral", "skipped":
			return CIStatusPassed
		case "pending", "queued", "in_progress":
			return CIStatusRunning
		case "failure", "error", "cancelled", "timed_out", "action_required", "stale":
			return CIStatusFailed
		}
	case "gitlab":
		// Pipeline status: created | waiting_for_resource | preparing |
		// pending | running | success | failed | canceled | skipped |
		// manual | scheduled.
		switch raw {
		case "success":
			return CIStatusPassed
		case "created", "waiting_for_resource", "preparing", "pending", "running", "manual", "scheduled":
			return CIStatusRunning
		case "failed", "canceled":
			return CIStatusFailed
		case "skipped":
			return CIStatusPassed
		}
	case "azure_devops":
		// Build status: notStarted | inProgress | completed.
		// Build result (when completed): succeeded | partiallySucceeded |
		// failed | canceled.
		switch raw {
		case "succeeded", "partiallysucceeded":
			return CIStatusPassed
		case "notstarted", "inprogress":
			return CIStatusRunning
		case "failed", "canceled":
			return CIStatusFailed
		}
	case "bitbucket":
		// Reserved for v2 — no Bitbucket CI yet.
		return CIStatusNone
	}

	return CIStatusFailed
}
