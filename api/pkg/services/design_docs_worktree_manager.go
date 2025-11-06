package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/rs/zerolog/log"
)

// DesignDocsWorktreeManager manages git worktrees for helix-design-docs branch
// This branch is forward-only and survives git operations on main code
type DesignDocsWorktreeManager struct {
	gitUserName  string
	gitUserEmail string
}

// TaskItem represents a single task in the progress checklist
type TaskItem struct {
	Index       int        `json:"index"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"` // pending, in_progress, completed
	RawLine     string     `json:"raw_line"`
	LineNumber  int        `json:"line_number"`
}

// TaskStatus represents the status of a task
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
)

// NewDesignDocsWorktreeManager creates a new worktree manager
func NewDesignDocsWorktreeManager(gitUserName, gitUserEmail string) *DesignDocsWorktreeManager {
	return &DesignDocsWorktreeManager{
		gitUserName:  gitUserName,
		gitUserEmail: gitUserEmail,
	}
}

// SetupWorktree creates the helix-design-docs branch and worktree if not exists
func (m *DesignDocsWorktreeManager) SetupWorktree(ctx context.Context, repoPath string) (string, error) {
	log.Info().
		Str("repo_path", repoPath).
		Msg("Setting up helix-design-docs worktree")

	// Open git repository
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}

	// Check if helix-design-docs branch exists
	branchName := "helix-design-docs"
	branchRefName := plumbing.NewBranchReferenceName(branchName)

	_, err = repo.Reference(branchRefName, true)
	if err == plumbing.ErrReferenceNotFound {
		// Branch doesn't exist, create it
		err = m.createDesignDocsBranch(repo, branchRefName)
		if err != nil {
			return "", fmt.Errorf("failed to create helix-design-docs branch: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("failed to check for branch: %w", err)
	}

	// Setup worktree
	worktreePath := filepath.Join(repoPath, ".git-worktrees", "helix-design-docs")

	// Check if worktree already exists
	if _, err := os.Stat(worktreePath); err == nil {
		log.Info().
			Str("worktree_path", worktreePath).
			Msg("Worktree already exists")
		return worktreePath, nil
	}

	// Create worktree directory
	err = os.MkdirAll(filepath.Dir(worktreePath), 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create worktree parent directory: %w", err)
	}

	// Note: go-git doesn't have direct worktree add support
	// We'll use a simpler approach: clone the branch to the worktree path
	err = m.setupWorktreeDirectory(repo, branchRefName, worktreePath)
	if err != nil {
		return "", fmt.Errorf("failed to setup worktree directory: %w", err)
	}

	// Initialize with template structure
	err = m.initializeTemplates(worktreePath)
	if err != nil {
		return "", fmt.Errorf("failed to initialize templates: %w", err)
	}

	log.Info().
		Str("worktree_path", worktreePath).
		Msg("Successfully setup helix-design-docs worktree")

	return worktreePath, nil
}

// createDesignDocsBranch creates the helix-design-docs branch
func (m *DesignDocsWorktreeManager) createDesignDocsBranch(repo *git.Repository, branchRef plumbing.ReferenceName) error {
	// Get HEAD reference
	headRef, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Create new branch reference pointing to HEAD commit
	ref := plumbing.NewHashReference(branchRef, headRef.Hash())
	err = repo.Storer.SetReference(ref)
	if err != nil {
		return fmt.Errorf("failed to create branch reference: %w", err)
	}

	log.Info().
		Str("branch", branchRef.Short()).
		Str("commit", headRef.Hash().String()[:8]).
		Msg("Created helix-design-docs branch")

	return nil
}

// setupWorktreeDirectory sets up the worktree directory manually
func (m *DesignDocsWorktreeManager) setupWorktreeDirectory(repo *git.Repository, branchRef plumbing.ReferenceName, worktreePath string) error {
	// Create worktree directory
	err := os.MkdirAll(worktreePath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create worktree directory: %w", err)
	}

	// Get the branch reference
	ref, err := repo.Reference(branchRef, true)
	if err != nil {
		return fmt.Errorf("failed to get branch reference: %w", err)
	}

	// Get commit for the branch
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return fmt.Errorf("failed to get commit: %w", err)
	}

	// Get tree for the commit
	tree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("failed to get tree: %w", err)
	}

	// Checkout files to worktree directory
	err = m.checkoutTree(tree, worktreePath)
	if err != nil {
		return fmt.Errorf("failed to checkout tree: %w", err)
	}

	// Note: For simplicity, we'll initialize as a separate git repo for the design docs
	// This avoids complexity with go-git's limited worktree support
	// The design docs will have their own git history on helix-design-docs branch
	_, err = git.PlainInit(worktreePath, false)
	if err != nil {
		return fmt.Errorf("failed to init design docs repo: %w", err)
	}

	return nil
}

// checkoutTree checks out a git tree to a directory
func (m *DesignDocsWorktreeManager) checkoutTree(tree *object.Tree, targetPath string) error {
	return tree.Files().ForEach(func(f *object.File) error {
		filePath := filepath.Join(targetPath, f.Name)

		// Create directory if needed
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		// Get file contents
		reader, err := f.Reader()
		if err != nil {
			return err
		}
		defer reader.Close()

		// Write file
		content := make([]byte, f.Size)
		_, err = reader.Read(content)
		if err != nil {
			return err
		}

		return os.WriteFile(filePath, content, 0644)
	})
}

// initializeTemplates creates initial design doc structure with proper organization
func (m *DesignDocsWorktreeManager) initializeTemplates(worktreePath string) error {
	// Create README explaining structure
	readmeTemplate := `# Helix Design Documents

This directory contains design documents and progress tracking for SpecTasks.
All documents here are managed by Helix agents and the orchestrator.

## Directory Structure

` + "```" + `
helix-design-docs/
├── README.md                    (this file)
├── tasks/                       (organized by date + branch name)
│   ├── 2025-10-08_add-user-auth_spec_abc123/
│   │   ├── requirements.md     (user stories + EARS acceptance criteria)
│   │   ├── design.md          (architecture + sequence diagrams)
│   │   ├── tasks.md           (discrete implementation tasks)
│   │   └── sessions/
│   │       ├── ses_planning_xyz.md
│   │       └── ses_impl_abc.md
│   └── 2025-10-09_fix-api-bug_spec_def456/
│       ├── requirements.md
│       └── ...
└── archive/                     (completed tasks)
    └── 2025-10/
        └── 2025-10-01_feature_spec_old123/
` + "```" + `

## File Naming Convention (Following spec-driven development)

- **Task directories**: ` + "`{YYYY-MM-DD}_{branch-name}_{task_id}/`" + `
  - Date first for sorting
  - Branch name for readability
  - Task ID for uniqueness
- **requirements.md** - User stories + EARS acceptance criteria
- **design.md** - Architecture + sequence diagrams + implementation considerations
- **tasks.md** - Discrete, trackable implementation tasks (with [ ]/[~]/[x] markers)
- **sessions/** - Per-session notes (` + "`ses_{session_id}.md`" + `)

## Task Status Markers

In tasks.md (following spec-driven development):
- ` + "`- [ ]`" + ` Pending task
- ` + "`- [~]`" + ` In progress (currently working)
- ` + "`- [x]`" + ` Completed

## Git Workflow

All changes committed to helix-design-docs branch.
This branch is **forward-only** and never rolled back.

---
*Managed by Helix SpecTask Orchestrator*
`

	err := os.WriteFile(filepath.Join(worktreePath, "README.md"), []byte(readmeTemplate), 0644)
	if err != nil {
		return fmt.Errorf("failed to create README.md: %w", err)
	}

	// Create directory structure
	tasksDir := filepath.Join(worktreePath, "tasks")
	err = os.MkdirAll(tasksDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create tasks directory: %w", err)
	}

	archiveDir := filepath.Join(worktreePath, "archive")
	err = os.MkdirAll(archiveDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	return nil
}

// InitializeTaskDirectory creates a properly organized directory for a specific SpecTask
func (m *DesignDocsWorktreeManager) InitializeTaskDirectory(worktreePath string, taskID string, taskName string) (string, error) {
	// Generate branch-style name from task name
	branchName := sanitizeForBranchName(taskName)

	// Generate directory name: {YYYY-MM-DD}_{branch-name}_{task_id}
	dateStr := time.Now().Format("2006-01-02")
	taskDirName := fmt.Sprintf("%s_%s_%s", dateStr, branchName, taskID)
	taskDir := filepath.Join(worktreePath, "tasks", taskDirName)

	// Create task directory
	err := os.MkdirAll(taskDir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create task directory: %w", err)
	}

	// Create requirements.md template (EARS notation format)
	requirementsTemplate := fmt.Sprintf(`# Requirements

**SpecTask ID**: %s
**Created**: %s

## User Stories

As a [user type], I want [goal] so that [benefit]

## Acceptance Criteria (EARS Notation)

WHEN [condition/event]
THE SYSTEM SHALL [expected behavior]

---
*Scale complexity to match the task - simple tasks need minimal detail, complex tasks need more*
`, taskID, time.Now().Format("2006-01-02"))

	err = os.WriteFile(filepath.Join(taskDir, "requirements.md"), []byte(requirementsTemplate), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create requirements.md: %w", err)
	}

	// Create design.md template (technical architecture format)
	designTemplate := fmt.Sprintf(`# Design

**SpecTask ID**: %s
**Created**: %s

## Architecture

Brief overview of system components

## Key Technical Decisions

Critical choices and rationale

---
*For complex tasks, add sections like: Data Models, Sequence Diagrams, API Contracts, Error Handling*
`, taskID, time.Now().Format("2006-01-02"))

	err = os.WriteFile(filepath.Join(taskDir, "design.md"), []byte(designTemplate), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create design.md: %w", err)
	}

	// Create tasks.md template (discrete task breakdown)
	tasksTemplate := fmt.Sprintf(`# Tasks

**SpecTask ID**: %s
**Created**: %s

## Implementation Tasks

- [ ] Task 1: Brief description
- [ ] Task 2: Brief description

---
*Number of tasks depends on scope - could be 2 tasks or 30+ for complex work*
`, taskID, time.Now().Format("2006-01-02"))

	err = os.WriteFile(filepath.Join(taskDir, "tasks.md"), []byte(tasksTemplate), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create tasks.md: %w", err)
	}

	// Create sessions subdirectory
	sessionsDir := filepath.Join(taskDir, "sessions")
	err = os.MkdirAll(sessionsDir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create sessions directory: %w", err)
	}

	log.Info().
		Str("task_id", taskID).
		Str("task_dir", taskDir).
		Msg("Initialized organized task directory in helix-design-docs")

	return taskDir, nil
}

// GetCurrentTask finds the task currently in progress (marked with [~])
func (m *DesignDocsWorktreeManager) GetCurrentTask(worktreePath string) (*TaskItem, error) {
	tasks, err := m.ParseTaskList(worktreePath)
	if err != nil {
		return nil, err
	}

	for _, task := range tasks {
		if task.Status == TaskStatusInProgress {
			return &task, nil
		}
	}

	return nil, nil // No task in progress
}

// GetNextPendingTask finds the next pending task to work on
func (m *DesignDocsWorktreeManager) GetNextPendingTask(worktreePath string) (*TaskItem, error) {
	tasks, err := m.ParseTaskList(worktreePath)
	if err != nil {
		return nil, err
	}

	for _, task := range tasks {
		if task.Status == TaskStatusPending {
			return &task, nil
		}
	}

	return nil, nil // No pending tasks
}

// ParseTaskList parses the task list from tasks.md
func (m *DesignDocsWorktreeManager) ParseTaskList(worktreePath string) ([]TaskItem, error) {
	tasksPath := filepath.Join(worktreePath, "tasks.md")
	content, err := os.ReadFile(tasksPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read tasks.md: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	tasks := []TaskItem{}
	taskIndex := 0

	// Regex patterns for different task states
	pendingPattern := regexp.MustCompile(`^[-*]\s+\[\s\]\s+(.+)$`)
	inProgressPattern := regexp.MustCompile(`^[-*]\s+\[~\]\s+(.+)$`)
	completedPattern := regexp.MustCompile(`^[-*]\s+\[x\]\s+(.+)$`)

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		var status TaskStatus
		var description string

		if matches := pendingPattern.FindStringSubmatch(line); matches != nil {
			status = TaskStatusPending
			description = matches[1]
		} else if matches := inProgressPattern.FindStringSubmatch(line); matches != nil {
			status = TaskStatusInProgress
			description = matches[1]
		} else if matches := completedPattern.FindStringSubmatch(line); matches != nil {
			status = TaskStatusCompleted
			description = matches[1]
		} else {
			continue // Not a task line
		}

		tasks = append(tasks, TaskItem{
			Index:       taskIndex,
			Description: description,
			Status:      status,
			RawLine:     line,
			LineNumber:  lineNum,
		})
		taskIndex++
	}

	return tasks, nil
}

// MarkTaskInProgress marks a task as in-progress and commits to git
func (m *DesignDocsWorktreeManager) MarkTaskInProgress(ctx context.Context, worktreePath string, taskIndex int) error {
	tasks, err := m.ParseTaskList(worktreePath)
	if err != nil {
		return err
	}

	if taskIndex >= len(tasks) {
		return fmt.Errorf("task index %d out of bounds (total tasks: %d)", taskIndex, len(tasks))
	}

	task := tasks[taskIndex]

	// Update task status in file
	err = m.updateTaskStatus(worktreePath, task.LineNumber, "~")
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	// Commit change
	commitMsg := fmt.Sprintf("🤖 Agent: Started task %d - %s", taskIndex, task.Description)
	err = m.commitChanges(worktreePath, "progress.md", commitMsg)
	if err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	log.Info().
		Int("task_index", taskIndex).
		Str("description", task.Description).
		Msg("Marked task as in-progress")

	return nil
}

// MarkTaskComplete marks a task as completed and commits to git
func (m *DesignDocsWorktreeManager) MarkTaskComplete(ctx context.Context, worktreePath string, taskIndex int) error {
	tasks, err := m.ParseTaskList(worktreePath)
	if err != nil {
		return err
	}

	if taskIndex >= len(tasks) {
		return fmt.Errorf("task index %d out of bounds (total tasks: %d)", taskIndex, len(tasks))
	}

	task := tasks[taskIndex]

	// Update task status in file
	err = m.updateTaskStatus(worktreePath, task.LineNumber, "x")
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	// Commit change
	commitMsg := fmt.Sprintf("🤖 Agent: Completed task %d - %s", taskIndex, task.Description)
	err = m.commitChanges(worktreePath, "progress.md", commitMsg)
	if err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	log.Info().
		Int("task_index", taskIndex).
		Str("description", task.Description).
		Msg("Marked task as completed")

	return nil
}

// updateTaskStatus updates the status marker in tasks.md
func (m *DesignDocsWorktreeManager) updateTaskStatus(worktreePath string, lineNumber int, statusChar string) error {
	tasksPath := filepath.Join(worktreePath, "tasks.md")
	content, err := os.ReadFile(tasksPath)
	if err != nil {
		return fmt.Errorf("failed to read tasks.md: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	if lineNumber >= len(lines) {
		return fmt.Errorf("line number %d out of bounds", lineNumber)
	}

	// Replace status marker: [ ] or [~] or [x]
	line := lines[lineNumber]
	updatedLine := regexp.MustCompile(`\[[\s~x]\]`).ReplaceAllString(line, fmt.Sprintf("[%s]", statusChar))
	lines[lineNumber] = updatedLine

	// Write back
	newContent := strings.Join(lines, "\n")
	err = os.WriteFile(tasksPath, []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write tasks.md: %w", err)
	}

	return nil
}

// commitChanges commits a file change to the helix-design-docs branch
func (m *DesignDocsWorktreeManager) commitChanges(worktreePath, filePath, message string) error {
	// Open git repository at worktree path
	repo, err := git.PlainOpen(worktreePath)
	if err != nil {
		return fmt.Errorf("failed to open worktree repository: %w", err)
	}

	// Get worktree
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add file
	_, err = worktree.Add(filePath)
	if err != nil {
		return fmt.Errorf("failed to add file: %w", err)
	}

	// Commit
	signature := &object.Signature{
		Name:  m.gitUserName,
		Email: m.gitUserEmail,
		When:  time.Now(),
	}

	_, err = worktree.Commit(message, &git.CommitOptions{
		Author:    signature,
		Committer: signature,
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	return nil
}

// GetTaskContext returns context for dashboard display (tasks before/after current)
func (m *DesignDocsWorktreeManager) GetTaskContext(worktreePath string, contextSize int) (*TaskContext, error) {
	tasks, err := m.ParseTaskList(worktreePath)
	if err != nil {
		return nil, err
	}

	// Find current task
	currentIndex := -1
	for i, task := range tasks {
		if task.Status == TaskStatusInProgress {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 {
		// No current task, return empty context
		return &TaskContext{
			CurrentTask:  nil,
			TasksBefore:  []TaskItem{},
			TasksAfter:   []TaskItem{},
			TotalTasks:   len(tasks),
			CompletedCount: countCompleted(tasks),
		}, nil
	}

	// Get tasks before
	beforeStart := currentIndex - contextSize
	if beforeStart < 0 {
		beforeStart = 0
	}
	tasksBefore := tasks[beforeStart:currentIndex]

	// Get tasks after
	afterEnd := currentIndex + 1 + contextSize
	if afterEnd > len(tasks) {
		afterEnd = len(tasks)
	}
	tasksAfter := tasks[currentIndex+1 : afterEnd]

	return &TaskContext{
		CurrentTask:    &tasks[currentIndex],
		TasksBefore:    tasksBefore,
		TasksAfter:     tasksAfter,
		TotalTasks:     len(tasks),
		CompletedCount: countCompleted(tasks),
	}, nil
}

// TaskContext provides context for dashboard display
type TaskContext struct {
	CurrentTask    *TaskItem   `json:"current_task"`
	TasksBefore    []TaskItem  `json:"tasks_before"`
	TasksAfter     []TaskItem  `json:"tasks_after"`
	TotalTasks     int         `json:"total_tasks"`
	CompletedCount int         `json:"completed_count"`
}

func countCompleted(tasks []TaskItem) int {
	count := 0
	for _, task := range tasks {
		if task.Status == TaskStatusCompleted {
			count++
		}
	}
	return count
}
