package services

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// sanitizeForBranchName converts a task name into a valid git branch-style name
// Splits on words and limits to 25 characters without cutting mid-word
// Examples:
//   "Add user authentication" → "add-user-authentication"
//   "Fix: API timeout issue" → "fix-api-timeout-issue"
//   "Install cowsay and make it work" → "install-cowsay-and-make"
func sanitizeForBranchName(taskName string) string {
	// Convert to lowercase
	name := strings.ToLower(taskName)

	// Remove special characters except hyphens and alphanumeric
	reg := regexp.MustCompile(`[^a-z0-9-\s]`)
	name = reg.ReplaceAllString(name, "")

	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")

	// Replace multiple hyphens with single hyphen
	reg = regexp.MustCompile(`-+`)
	name = reg.ReplaceAllString(name, "-")

	// Trim hyphens from start and end
	name = strings.Trim(name, "-")

	// Limit to 25 characters, but split on word boundaries (hyphens)
	if len(name) > 25 {
		// Find the last hyphen before the 25-char limit
		truncated := name[:25]
		lastHyphen := strings.LastIndex(truncated, "-")
		if lastHyphen > 10 { // Keep at least 10 chars
			name = truncated[:lastHyphen]
		} else {
			name = truncated
		}
	}

	// Trim any trailing hyphen after truncation
	name = strings.TrimRight(name, "-")

	return name
}

// GenerateDesignDocPath creates a human-readable directory path for design docs
// Format: "YYYY-MM-DD_shortname_N" e.g., "2025-12-09_install-cowsay_1"
// The taskNumber should come from atomically incrementing project.NextTaskNumber
func GenerateDesignDocPath(task *types.SpecTask, taskNumber int) string {
	dateStr := time.Now().Format("2006-01-02")
	sanitizedName := sanitizeForBranchName(task.Name) // Already limited to 25 chars
	return fmt.Sprintf("%s_%s_%d", dateStr, sanitizedName, taskNumber)
}

// GenerateFeatureBranchName creates a human-readable feature branch name
// Format: "feature/shortname-N" e.g., "feature/install-cowsay-123"
// Uses task.TaskNumber if set, otherwise falls back to last 8 chars of task ID
func GenerateFeatureBranchName(task *types.SpecTask) string {
	sanitizedName := sanitizeForBranchName(task.Name)

	// Use TaskNumber if available (new format), otherwise use ID suffix (backwards compat)
	if task.TaskNumber > 0 {
		return fmt.Sprintf("feature/%s-%d", sanitizedName, task.TaskNumber)
	}

	// Fallback for old tasks without TaskNumber
	taskIDSuffix := task.ID
	if len(taskIDSuffix) > 8 {
		taskIDSuffix = taskIDSuffix[len(taskIDSuffix)-8:]
	}
	return fmt.Sprintf("feature/%s-%s", sanitizedName, taskIDSuffix)
}
