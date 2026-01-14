package services

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

// TaskProgressItem represents a single task in the progress checklist
type TaskProgressItem struct {
	Index       int                `json:"index"`
	Description string             `json:"description"`
	Status      TaskProgressStatus `json:"status"` // pending, in_progress, completed
}

// TaskProgressStatus represents the status of a task
type TaskProgressStatus string

const (
	TaskProgressPending    TaskProgressStatus = "pending"
	TaskProgressInProgress TaskProgressStatus = "in_progress"
	TaskProgressCompleted  TaskProgressStatus = "completed"
)

// TaskProgress represents the overall progress of a SpecTask
type TaskProgress struct {
	Tasks          []TaskProgressItem `json:"tasks"`
	TotalTasks     int                `json:"total_tasks"`
	CompletedTasks int                `json:"completed_tasks"`
	InProgressTask *TaskProgressItem  `json:"in_progress_task,omitempty"`
	ProgressPct    int                `json:"progress_pct"` // 0-100
}

// ParseTaskProgress reads tasks.md from helix-specs branch and returns progress
// repoPath is the path to the git repository
// specTaskID is used to find the task directory (fallback for old tasks)
// designDocPath is the new human-readable path (e.g., "2025-12-09_install-cowsay_1")
func ParseTaskProgress(repoPath string, specTaskID string, designDocPath string) (*TaskProgress, error) {
	// Find task directory in helix-specs branch
	taskDir, err := findTaskDirectory(repoPath, specTaskID, designDocPath)
	if err != nil {
		log.Debug().Err(err).Str("spec_task_id", specTaskID).Str("design_doc_path", designDocPath).Msg("Could not find task directory in helix-specs")
		return nil, err
	}

	// Read tasks.md from helix-specs branch
	tasksPath := fmt.Sprintf("%s/tasks.md", taskDir)
	content, err := readFileFromBranch(repoPath, "helix-specs", tasksPath)
	if err != nil {
		log.Debug().Err(err).Str("path", tasksPath).Msg("Could not read tasks.md from helix-specs")
		return nil, err
	}

	// Parse the task list
	tasks := parseTaskList(content)

	// Calculate progress
	progress := &TaskProgress{
		Tasks:      tasks,
		TotalTasks: len(tasks),
	}

	for i := range tasks {
		switch tasks[i].Status {
		case TaskProgressCompleted:
			progress.CompletedTasks++
		case TaskProgressInProgress:
			progress.InProgressTask = &tasks[i]
		}
	}

	if progress.TotalTasks > 0 {
		progress.ProgressPct = (progress.CompletedTasks * 100) / progress.TotalTasks
	}

	return progress, nil
}

// findTaskDirectory finds the task directory in helix-specs branch
// First tries to match by designDocPath (new format: "2025-12-09_install-cowsay_1")
// Falls back to matching by specTaskID for backwards compatibility with old tasks
func findTaskDirectory(repoPath string, specTaskID string, designDocPath string) (string, error) {
	// List files in helix-specs branch
	cmd := exec.Command("git", "ls-tree", "--name-only", "-r", "helix-specs")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to list helix-specs branch: %w (%s)", err, string(output))
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")

	// First, try to find by designDocPath (new human-readable format)
	if designDocPath != "" {
		for _, file := range files {
			if strings.Contains(file, designDocPath) {
				parts := strings.Split(file, "/")
				if len(parts) >= 2 {
					return strings.Join(parts[:len(parts)-1], "/"), nil
				}
			}
		}
	}

	// Fall back to matching by specTaskID for backwards compatibility
	for _, file := range files {
		if strings.Contains(file, specTaskID) {
			// Extract directory path (e.g., tasks/2025-11-11_..._taskid/)
			parts := strings.Split(file, "/")
			if len(parts) >= 2 {
				// Return the directory containing the task files
				return strings.Join(parts[:len(parts)-1], "/"), nil
			}
		}
	}

	return "", fmt.Errorf("task directory not found for %s (designDocPath: %s)", specTaskID, designDocPath)
}

// readFileFromBranch reads a file from a specific git branch
func readFileFromBranch(repoPath, branch, filePath string) (string, error) {
	cmd := exec.Command("git", "show", fmt.Sprintf("%s:%s", branch, filePath))
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to read %s from %s: %w", filePath, branch, err)
	}
	return string(output), nil
}

// parseTaskList parses markdown task list into TaskProgressItems
func parseTaskList(content string) []TaskProgressItem {
	lines := strings.Split(content, "\n")
	tasks := []TaskProgressItem{}
	taskIndex := 0

	// Regex patterns for different task states
	// Matches: - [ ] task, * [ ] task, - [x] task, - [~] task
	pendingPattern := regexp.MustCompile(`^[-*]\s+\[\s\]\s+(.+)$`)
	inProgressPattern := regexp.MustCompile(`^[-*]\s+\[~\]\s+(.+)$`)
	completedPattern := regexp.MustCompile(`^[-*]\s+\[[xX]\]\s+(.+)$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		var status TaskProgressStatus
		var description string

		if matches := pendingPattern.FindStringSubmatch(line); matches != nil {
			status = TaskProgressPending
			description = matches[1]
		} else if matches := inProgressPattern.FindStringSubmatch(line); matches != nil {
			status = TaskProgressInProgress
			description = matches[1]
		} else if matches := completedPattern.FindStringSubmatch(line); matches != nil {
			status = TaskProgressCompleted
			description = matches[1]
		} else {
			continue // Not a task line
		}

		tasks = append(tasks, TaskProgressItem{
			Index:       taskIndex,
			Description: description,
			Status:      status,
		})
		taskIndex++
	}

	return tasks
}
