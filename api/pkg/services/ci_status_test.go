package services

import "testing"

func TestNormalizeCIStatus(t *testing.T) {
	cases := []struct {
		provider string
		raw      string
		want     string
	}{
		// Empty raw is always "none".
		{"github", "", CIStatusNone},
		{"gitlab", "", CIStatusNone},
		{"azure_devops", "", CIStatusNone},
		{"bitbucket", "", CIStatusNone},

		// GitHub combined status + check run conclusions.
		{"github", "success", CIStatusPassed},
		{"github", "neutral", CIStatusPassed},
		{"github", "skipped", CIStatusPassed},
		{"github", "pending", CIStatusRunning},
		{"github", "queued", CIStatusRunning},
		{"github", "in_progress", CIStatusRunning},
		{"github", "failure", CIStatusFailed},
		{"github", "error", CIStatusFailed},
		{"github", "cancelled", CIStatusFailed},
		{"github", "timed_out", CIStatusFailed},
		{"github", "action_required", CIStatusFailed},
		{"github", "stale", CIStatusFailed},
		// Case- and whitespace-insensitive.
		{"github", "  SUCCESS ", CIStatusPassed},

		// GitLab pipeline statuses.
		{"gitlab", "success", CIStatusPassed},
		{"gitlab", "running", CIStatusRunning},
		{"gitlab", "pending", CIStatusRunning},
		{"gitlab", "preparing", CIStatusRunning},
		{"gitlab", "manual", CIStatusRunning},
		{"gitlab", "scheduled", CIStatusRunning},
		{"gitlab", "failed", CIStatusFailed},
		{"gitlab", "canceled", CIStatusFailed},
		{"gitlab", "skipped", CIStatusPassed},

		// Azure DevOps build status / result.
		{"azure_devops", "succeeded", CIStatusPassed},
		{"azure_devops", "partiallySucceeded", CIStatusPassed},
		{"azure_devops", "inProgress", CIStatusRunning},
		{"azure_devops", "notStarted", CIStatusRunning},
		{"azure_devops", "failed", CIStatusFailed},
		{"azure_devops", "canceled", CIStatusFailed},

		// Bitbucket: always "none" in v1.
		{"bitbucket", "SUCCESSFUL", CIStatusNone},

		// Unknown raw values surface as "failed" rather than swallowed.
		{"github", "lolwut", CIStatusFailed},
		{"gitlab", "lolwut", CIStatusFailed},
	}

	for _, c := range cases {
		got := NormalizeCIStatus(c.provider, c.raw)
		if got != c.want {
			t.Errorf("NormalizeCIStatus(%q, %q) = %q, want %q",
				c.provider, c.raw, got, c.want)
		}
	}
}
