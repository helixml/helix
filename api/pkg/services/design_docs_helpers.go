package services

import (
	"regexp"
	"strings"
)

// sanitizeForBranchName converts a task name into a valid git branch-style name
// Examples:
//   "Add user authentication" → "add-user-authentication"
//   "Fix: API timeout issue" → "fix-api-timeout-issue"
//   "Implement OAuth 2.0" → "implement-oauth-2-0"
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

	// Limit length to 50 characters for readability
	if len(name) > 50 {
		name = name[:50]
	}

	// Trim any trailing hyphen after truncation
	name = strings.TrimRight(name, "-")

	return name
}
