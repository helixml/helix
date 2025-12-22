package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

// DesignDocStore is a minimal interface for uniqueness checks
type DesignDocStore interface {
	ListSpecTasks(ctx context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error)
}

// sanitizeForBranchName converts a task name into a valid git branch-style name
// Splits on words and limits to 25 characters without cutting mid-word
// Examples:
//
//	"Add user authentication" → "add-user-authentication"
//	"Fix: API timeout issue" → "fix-api-timeout-issue"
//	"Install cowsay and make it work" → "install-cowsay-and-make"
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
// Format: "NNNNNN_shortname" e.g., "000001_install-cowsay"
// The taskNumber is globally unique across the entire deployment
func GenerateDesignDocPath(task *types.SpecTask, taskNumber int) string {
	sanitizedName := sanitizeForBranchName(task.Name) // Already limited to 25 chars
	return fmt.Sprintf("%06d_%s", taskNumber, sanitizedName)
}

// GenerateFeatureBranchName creates a human-readable feature branch name
// Format: "feature/NNNNNN-shortname" e.g., "feature/000001-install-cowsay"
// If task.BranchPrefix is set by user, uses that instead of auto-generating from name
// Uses task.TaskNumber if set, otherwise falls back to last 8 chars of task ID
func GenerateFeatureBranchName(task *types.SpecTask) string {
	sanitizedName := sanitizeForBranchName(task.Name)

	// Use user-specified prefix if provided
	if task.BranchPrefix != "" {
		// User provided a prefix like "feature/user-auth" - sanitize it
		baseName := sanitizeBranchPrefix(task.BranchPrefix)
		// Use TaskNumber if available (new format), otherwise use ID suffix (backwards compat)
		if task.TaskNumber > 0 {
			return fmt.Sprintf("%s-%06d", baseName, task.TaskNumber)
		}
		// Fallback for old tasks without TaskNumber
		taskIDSuffix := task.ID
		if len(taskIDSuffix) > 8 {
			taskIDSuffix = taskIDSuffix[len(taskIDSuffix)-8:]
		}
		return fmt.Sprintf("%s-%s", baseName, taskIDSuffix)
	}

	// Auto-generate: feature/NNNNNN-shortname
	if task.TaskNumber > 0 {
		return fmt.Sprintf("feature/%06d-%s", task.TaskNumber, sanitizedName)
	}

	// Fallback for old tasks without TaskNumber
	taskIDSuffix := task.ID
	if len(taskIDSuffix) > 8 {
		taskIDSuffix = taskIDSuffix[len(taskIDSuffix)-8:]
	}
	return fmt.Sprintf("feature/%s-%s", taskIDSuffix, sanitizedName)
}

// sanitizeBranchPrefix cleans up a user-provided branch prefix
// Allows forward slashes for namespacing like "feature/user-auth" or "fix/login-bug"
func sanitizeBranchPrefix(prefix string) string {
	// Convert to lowercase
	name := strings.ToLower(prefix)

	// Remove special characters except hyphens, underscores, forward slashes, and alphanumeric
	reg := regexp.MustCompile(`[^a-z0-9-_/]`)
	name = reg.ReplaceAllString(name, "")

	// Replace multiple hyphens with single hyphen
	reg = regexp.MustCompile(`-+`)
	name = reg.ReplaceAllString(name, "-")

	// Replace multiple slashes with single slash
	reg = regexp.MustCompile(`/+`)
	name = reg.ReplaceAllString(name, "/")

	// Trim hyphens and slashes from start and end
	name = strings.Trim(name, "-/")

	return name
}

// GenerateUniqueDesignDocPath creates a design doc path for the task.
//
// TODO(2025-12-18): Uniqueness check disabled - same issue as GenerateUniqueBranchName.
// See that function's TODO for details. Re-enable after investigating root cause.
func GenerateUniqueDesignDocPath(ctx context.Context, store DesignDocStore, task *types.SpecTask, taskNumber int) (string, error) {
	basePath := GenerateDesignDocPath(task, taskNumber)

	// Uniqueness check disabled - see TODO above
	return basePath, nil

	/*
		// Check if this path exists (across all projects)
		tasks, err := store.ListSpecTasks(ctx, &types.SpecTaskFilters{
			DesignDocPath:   basePath,
			IncludeArchived: true,
		})
		if err != nil {
			return "", fmt.Errorf("failed to check design doc path uniqueness: %w", err)
		}

		// If no existing task with this path, use it
		if len(tasks) == 0 {
			return basePath, nil
		}

		// Path exists, try with suffixes _1, _2, etc.
		for suffix := 1; suffix <= 100; suffix++ {
			candidatePath := fmt.Sprintf("%s_%d", basePath, suffix)
			tasks, err := store.ListSpecTasks(ctx, &types.SpecTaskFilters{
				DesignDocPath:   candidatePath,
				IncludeArchived: true,
			})
			if err != nil {
				return "", fmt.Errorf("failed to check design doc path uniqueness: %w", err)
			}
			if len(tasks) == 0 {
				return candidatePath, nil
			}
		}

		// Fallback: use task ID for guaranteed uniqueness
		return fmt.Sprintf("%s_%s", basePath, task.ID[:8]), nil
	*/
}

// GenerateUniqueBranchName creates a branch name for the task.
//
// TODO(2025-12-18): Uniqueness check disabled due to observed issue:
// - Task spt_01kcrahm71zgkfmhdbp4a7y27n got branch name "feature/0001-make-the-homepage-of-the-1"
// - But querying the DB shows NO other task with the base name "feature/0001-make-the-homepage-of-the"
// - The LLM followed the prompt and created the branch it was told to, but PushBranchToRemote failed
// - Actual branch in git was "feature/0001-make-the-homepage-of-the" (no suffix)
//
// Root cause: The uniqueness check INCORRECTLY added "-1" when there was no collision.
// Possible causes:
// 1. Race condition - another task briefly had this branch name then was deleted/updated?
// 2. Bug in ListSpecTasks filter - maybe doing prefix match instead of exact match?
// 3. Stale data or caching issue?
//
// For now, just return the base branch name. Task numbers are per-project so
// collisions are unlikely in practice. Re-enable after investigating root cause.
func GenerateUniqueBranchName(ctx context.Context, store DesignDocStore, task *types.SpecTask) (string, error) {
	baseBranch := GenerateFeatureBranchName(task)

	// Uniqueness check disabled - see TODO above
	return baseBranch, nil

	/*
		// Check if this branch exists (across all projects)
		tasks, err := store.ListSpecTasks(ctx, &types.SpecTaskFilters{
			BranchName:      baseBranch,
			IncludeArchived: true,
		})
		if err != nil {
			return "", fmt.Errorf("failed to check branch name uniqueness: %w", err)
		}

		// If no existing task with this branch, use it
		if len(tasks) == 0 {
			return baseBranch, nil
		}

		// Branch exists, try with suffixes -1, -2, etc.
		for suffix := 1; suffix <= 100; suffix++ {
			candidateBranch := fmt.Sprintf("%s-%d", baseBranch, suffix)
			tasks, err := store.ListSpecTasks(ctx, &types.SpecTaskFilters{
				BranchName:      candidateBranch,
				IncludeArchived: true,
			})
			if err != nil {
				return "", fmt.Errorf("failed to check branch name uniqueness: %w", err)
			}
			if len(tasks) == 0 {
				return candidateBranch, nil
			}
		}

		// Fallback: use task ID for guaranteed uniqueness
		return fmt.Sprintf("%s-%s", baseBranch, task.ID[:8]), nil
	*/
}
