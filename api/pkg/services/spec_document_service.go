package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SpecDocumentService manages git-based spec document handoff following Kiro's approach
// Generates and manages requirements.md, design.md, and tasks.md files in git repositories
type SpecDocumentService struct {
	store         store.Store
	gitBasePath   string // Base path for git repositories
	gitUserName   string // Git user name for commits
	gitUserEmail  string // Git user email for commits
	defaultBranch string // Default branch for spec documents (e.g., "main")
	specBranch    string // Branch for spec documents (e.g., "specs")
	testMode      bool   // If true, skip actual git operations
}

// SpecDocumentConfig represents configuration for spec document generation
type SpecDocumentConfig struct {
	SpecTaskID        string            `json:"spec_task_id"`
	ProjectPath       string            `json:"project_path"`
	BranchName        string            `json:"branch_name,omitempty"`
	CommitMessage     string            `json:"commit_message,omitempty"`
	IncludeTimestamps bool              `json:"include_timestamps"`
	GenerateTaskBoard bool              `json:"generate_task_board"`
	CustomMetadata    map[string]string `json:"custom_metadata,omitempty"`
	OverwriteExisting bool              `json:"overwrite_existing"`
	CreatePullRequest bool              `json:"create_pull_request"`
	ReviewersNeeded   []string          `json:"reviewers_needed,omitempty"`
}

// SpecDocumentResult represents the result of spec document operations
type SpecDocumentResult struct {
	SpecTaskID     string                 `json:"spec_task_id"`
	CommitHash     string                 `json:"commit_hash"`
	BranchName     string                 `json:"branch_name"`
	FilesCreated   []string               `json:"files_created"`
	PullRequestURL string                 `json:"pull_request_url,omitempty"`
	GeneratedFiles map[string]string      `json:"generated_files"` // filename -> content
	Warnings       []string               `json:"warnings,omitempty"`
	Success        bool                   `json:"success"`
	Message        string                 `json:"message"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// NewSpecDocumentService creates a new spec document service
func NewSpecDocumentService(
	store store.Store,
	gitBasePath string,
	gitUserName string,
	gitUserEmail string,
) *SpecDocumentService {
	return &SpecDocumentService{
		store:         store,
		gitBasePath:   gitBasePath,
		gitUserName:   gitUserName,
		gitUserEmail:  gitUserEmail,
		defaultBranch: "main",
		specBranch:    "specs",
		testMode:      false,
	}
}

// SetTestMode enables or disables test mode
func (s *SpecDocumentService) SetTestMode(enabled bool) {
	s.testMode = enabled
}

// GenerateSpecDocuments creates Kiro-style spec documents from approved SpecTask
func (s *SpecDocumentService) GenerateSpecDocuments(
	ctx context.Context,
	config *SpecDocumentConfig,
) (*SpecDocumentResult, error) {
	// Get SpecTask
	specTask, err := s.store.GetSpecTask(ctx, config.SpecTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get SpecTask: %w", err)
	}

	// Validate that specs are approved
	if specTask.Status != types.TaskStatusSpecApproved {
		return nil, fmt.Errorf("SpecTask must be approved before generating documents (current status: %s)", specTask.Status)
	}

	log.Info().
		Str("spec_task_id", config.SpecTaskID).
		Str("project_path", config.ProjectPath).
		Bool("test_mode", s.testMode).
		Msg("Generating Kiro-style spec documents")

	// Generate the three core documents
	generatedFiles := make(map[string]string)

	// 1. requirements.md - User stories with EARS notation
	requirementsContent := s.generateRequirementsMarkdown(specTask, config)
	generatedFiles["requirements.md"] = requirementsContent

	// 2. design.md - Technical architecture and design
	designContent := s.generateDesignMarkdown(specTask, config)
	generatedFiles["design.md"] = designContent

	// 3. tasks.md - Implementation plan with trackable tasks
	tasksContent := s.generateTasksMarkdown(specTask, config)
	generatedFiles["tasks.md"] = tasksContent

	// 4. Optional: spec-metadata.json for tooling integration
	metadataContent := s.GenerateSpecMetadata(specTask, config)
	generatedFiles["spec-metadata.json"] = metadataContent

	result := &SpecDocumentResult{
		SpecTaskID:     config.SpecTaskID,
		GeneratedFiles: generatedFiles,
		FilesCreated:   []string{"requirements.md", "design.md", "tasks.md", "spec-metadata.json"},
		Success:        true,
		Message:        "Spec documents generated successfully",
	}

	// Write to git repository (unless in test mode)
	if !s.testMode {
		commitResult, err := s.commitSpecDocuments(ctx, config, generatedFiles, specTask)
		if err != nil {
			return nil, fmt.Errorf("failed to commit spec documents: %w", err)
		}

		result.CommitHash = commitResult.CommitHash
		result.BranchName = commitResult.BranchName
		result.PullRequestURL = commitResult.PullRequestURL
		result.Warnings = commitResult.Warnings
	}

	log.Info().
		Str("spec_task_id", config.SpecTaskID).
		Str("commit_hash", result.CommitHash).
		Int("files_created", len(result.FilesCreated)).
		Msg("Successfully generated and committed spec documents")

	return result, nil
}

// generateRequirementsMarkdown creates requirements.md following Kiro's EARS notation
func (s *SpecDocumentService) generateRequirementsMarkdown(
	specTask *types.SpecTask,
	config *SpecDocumentConfig,
) string {
	var content strings.Builder

	// Header with metadata
	content.WriteString(fmt.Sprintf(`# Requirements Specification: %s

> **Generated from SpecTask**: %s
> **Created**: %s
> **Status**: %s
> **Planning Agent**: %s

## Overview

%s

## User Stories and Acceptance Criteria

The following requirements are written in EARS (Easy Approach to Requirements Syntax) notation for clarity and testability.

`,
		specTask.Name,
		specTask.ID,
		specTask.CreatedAt.Format("2006-01-02 15:04:05"),
		specTask.Status,
		specTask.SpecAgent,
		specTask.Description,
	))

	// Convert the requirements spec to EARS notation
	content.WriteString(s.convertToEARSNotation(specTask.RequirementsSpec))

	// Add traceability section
	content.WriteString(`

## Traceability

| Requirement ID | User Story | Implementation Task | Test Case |
|----------------|------------|-------------------|-----------|
`)

	// Parse implementation tasks for traceability
	implementationTasks, _ := s.store.ListSpecTaskImplementationTasks(context.Background(), specTask.ID)
	for i, task := range implementationTasks {
		content.WriteString(fmt.Sprintf("| REQ-%03d | %s | %s | TEST-%03d |\n",
			i+1,
			s.extractUserStoryFromTask(task),
			task.Title,
			i+1,
		))
	}

	// Add approval information
	if specTask.SpecApprovedBy != "" && specTask.SpecApprovedAt != nil {
		content.WriteString(fmt.Sprintf(`

## Approval

- **Approved by**: %s
- **Approved on**: %s
- **Revision count**: %d

`,
			specTask.SpecApprovedBy,
			specTask.SpecApprovedAt.Format("2006-01-02 15:04:05"),
			specTask.SpecRevisionCount,
		))
	}

	return content.String()
}

// generateDesignMarkdown creates design.md with technical architecture
func (s *SpecDocumentService) generateDesignMarkdown(
	specTask *types.SpecTask,
	config *SpecDocumentConfig,
) string {
	var content strings.Builder

	content.WriteString(fmt.Sprintf(`# Technical Design: %s

> **Generated from SpecTask**: %s
> **Created**: %s
> **Implementation Agent**: %s

## Architecture Overview

%s

## System Design

`,
		specTask.Name,
		specTask.ID,
		specTask.CreatedAt.Format("2006-01-02 15:04:05"),
		specTask.ImplementationAgent,
		specTask.Description,
	))

	// Add the technical design content
	content.WriteString(specTask.TechnicalDesign)

	// Add implementation context
	content.WriteString(`

## Implementation Context

### Multi-Session Approach

This specification will be implemented using a multi-session approach where:

- **Single Zed Instance**: All implementation work happens within one shared Zed instance
- **Multiple Work Sessions**: Each major component gets its own work session and Zed thread
- **Shared Project Context**: All sessions work on the same codebase with shared file access
- **Coordinated Development**: Sessions can spawn additional sessions and coordinate handoffs

### Session Architecture

`)

	// Add implementation tasks as session architecture
	implementationTasks, _ := s.store.ListSpecTaskImplementationTasks(context.Background(), specTask.ID)
	if len(implementationTasks) > 0 {
		content.WriteString("```\n")
		content.WriteString(fmt.Sprintf("SpecTask: \"%s\"\n", specTask.Name))
		content.WriteString("├── Planning Phase (Complete)\n")
		content.WriteString("└── Implementation Phase (Multi-Session)\n")
		content.WriteString("    ├── Zed Instance: Shared project context\n")
		for i, task := range implementationTasks {
			symbol := "├──"
			if i == len(implementationTasks)-1 {
				symbol = "└──"
			}
			content.WriteString(fmt.Sprintf("    %s WorkSession %d → Zed Thread (\"%s\")\n",
				symbol, i+1, task.Title))
		}
		content.WriteString("```\n")
	}

	// Add technical considerations
	content.WriteString(`

## Technical Considerations

### Development Environment
- **Shared Zed Instance**: All work sessions operate within the same Zed instance
- **Project Context**: Shared file access and project state across all threads
- **Coordination**: Inter-session communication through infrastructure layer
- **Resource Management**: Efficient use of development resources

### Quality Assurance
- **Specification Compliance**: All implementation must follow this approved design
- **Code Review**: Peer review process for significant changes
- **Testing Strategy**: Comprehensive testing at unit, integration, and system levels
- **Documentation**: Living documentation updated as implementation progresses

`)

	if config.IncludeTimestamps {
		content.WriteString(fmt.Sprintf(`

---
*Document generated on %s from SpecTask %s*
`, time.Now().Format("2006-01-02 15:04:05"), specTask.ID))
	}

	return content.String()
}

// generateTasksMarkdown creates tasks.md with implementation plan
func (s *SpecDocumentService) generateTasksMarkdown(
	specTask *types.SpecTask,
	config *SpecDocumentConfig,
) string {
	var content strings.Builder

	content.WriteString(fmt.Sprintf(`# Implementation Tasks: %s

> **Generated from SpecTask**: %s
> **Created**: %s
> **Implementation Agent**: %s

## Task Overview

This implementation plan breaks down the work into discrete, trackable tasks that will be executed across multiple coordinated work sessions.

`,
		specTask.Name,
		specTask.ID,
		specTask.CreatedAt.Format("2006-01-02 15:04:05"),
		specTask.ImplementationAgent,
	))

	// Get implementation tasks
	implementationTasks, err := s.store.ListSpecTaskImplementationTasks(context.Background(), specTask.ID)
	if err != nil {
		// Fall back to parsing the implementation plan
		content.WriteString("## Implementation Plan\n\n")
		content.WriteString(specTask.ImplementationPlan)
	} else {
		// Use parsed implementation tasks
		content.WriteString("## Implementation Tasks\n\n")

		for i, task := range implementationTasks {
			content.WriteString(fmt.Sprintf(`### Task %d: %s

**Description**: %s

**Estimated Effort**: %s
**Priority**: %d
**Status**: %s

`,
				i+1,
				task.Title,
				task.Description,
				task.EstimatedEffort,
				task.Priority,
				string(task.Status),
			))

			// Add acceptance criteria if available
			if task.AcceptanceCriteria != "" {
				content.WriteString("**Acceptance Criteria**:\n")
				content.WriteString(task.AcceptanceCriteria)
				content.WriteString("\n\n")
			}

			// Add dependencies if any
			if task.HasDependencies() {
				content.WriteString("**Dependencies**: ")
				var deps []int
				if err := json.Unmarshal(task.Dependencies, &deps); err == nil {
					depNames := make([]string, len(deps))
					for j, depIndex := range deps {
						if depIndex < len(implementationTasks) {
							depNames[j] = fmt.Sprintf("Task %d (%s)", depIndex+1, implementationTasks[depIndex].Title)
						} else {
							depNames[j] = fmt.Sprintf("Task %d", depIndex+1)
						}
					}
					content.WriteString(strings.Join(depNames, ", "))
				}
				content.WriteString("\n\n")
			}

			// Add work session assignment if available
			if task.AssignedWorkSessionID != "" {
				content.WriteString(fmt.Sprintf("**Assigned Work Session**: %s  \n", task.AssignedWorkSessionID))
			}

			content.WriteString("---\n\n")
		}
	}

	// Add multi-session coordination information
	content.WriteString(`## Multi-Session Coordination

### Session Management
- **Session Creation**: Work sessions will be created automatically for each implementation task
- **Zed Integration**: Each work session maps 1:1 to a Zed thread within the shared instance
- **Dynamic Spawning**: Additional work sessions can be spawned during implementation as needs emerge
- **Coordination**: Sessions coordinate through infrastructure-level services, not agent tools

### Progress Tracking
- **Real-time Status**: Track progress across all work sessions
- **Dependencies**: Respect task dependencies and blocking relationships
- **Completion Criteria**: SpecTask complete when all implementation tasks done
- **Quality Gates**: Code review and testing checkpoints

### Communication Patterns
- **Handoffs**: Clear handoff procedures between dependent tasks
- **Coordination Events**: Structured communication between work sessions
- **Escalation**: Human intervention points for complex decisions
- **Documentation**: All significant decisions recorded in git history

`)

	// Add execution timeline if configured
	if config.GenerateTaskBoard {
		content.WriteString(s.generateTaskBoard(implementationTasks))
	}

	if config.IncludeTimestamps {
		content.WriteString(fmt.Sprintf(`

---
*Task plan generated on %s from SpecTask %s*
*Implementation will be tracked in real-time with updates to this document*
`, time.Now().Format("2006-01-02 15:04:05"), specTask.ID))
	}

	return content.String()
}

// GenerateSpecMetadata creates spec-metadata.json for tooling integration
func (s *SpecDocumentService) GenerateSpecMetadata(
	specTask *types.SpecTask,
	config *SpecDocumentConfig,
) string {
	metadata := map[string]interface{}{
		"spec_task_id":         specTask.ID,
		"name":                 specTask.Name,
		"description":          specTask.Description,
		"type":                 specTask.Type,
		"priority":             specTask.Priority,
		"status":               specTask.Status,
		"created_by":           specTask.CreatedBy,
		"created_at":           specTask.CreatedAt,
		"updated_at":           specTask.UpdatedAt,
		"spec_approved_by":     specTask.SpecApprovedBy,
		"spec_approved_at":     specTask.SpecApprovedAt,
		"planning_agent":       specTask.SpecAgent,
		"implementation_agent": specTask.ImplementationAgent,
		"project_path":         specTask.ProjectPath,
		"zed_instance_id":      specTask.ZedInstanceID,
		"version":              "1.0",
		"format":               "kiro-style",
		"generated_at":         time.Now(),
		"multi_session":        true,
	}

	// Add custom metadata if provided
	if config.CustomMetadata != nil {
		for key, value := range config.CustomMetadata {
			metadata[key] = value
		}
	}

	// Add implementation task summary
	implementationTasks, err := s.store.ListSpecTaskImplementationTasks(context.Background(), specTask.ID)
	if err == nil {
		taskSummary := make([]map[string]interface{}, len(implementationTasks))
		for i, task := range implementationTasks {
			taskSummary[i] = map[string]interface{}{
				"index":            task.Index,
				"title":            task.Title,
				"estimated_effort": task.EstimatedEffort,
				"status":           string(task.Status),
				"assigned_session": task.AssignedWorkSessionID,
			}
		}
		metadata["implementation_tasks"] = taskSummary
	}

	// Serialize to formatted JSON
	jsonBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to marshal spec metadata")
		return "{}"
	}

	return string(jsonBytes)
}

// commitSpecDocuments commits the generated documents to git
func (s *SpecDocumentService) commitSpecDocuments(
	ctx context.Context,
	config *SpecDocumentConfig,
	generatedFiles map[string]string,
	specTask *types.SpecTask,
) (*SpecDocumentResult, error) {
	// Determine repository path
	repoPath := s.getRepositoryPath(config.ProjectPath, specTask)

	// Open or clone repository
	repo, err := s.openOrCreateRepository(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	// Create or checkout spec branch
	branchName := config.BranchName
	if branchName == "" {
		branchName = fmt.Sprintf("specs/%s", specTask.ID)
	}

	err = s.createOrCheckoutBranch(repo, branchName)
	if err != nil {
		return nil, fmt.Errorf("failed to create/checkout branch: %w", err)
	}

	// Create specs directory
	specsDir := filepath.Join(repoPath, "specs")
	specTaskDir := filepath.Join(specsDir, specTask.ID)

	err = os.MkdirAll(specTaskDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create specs directory: %w", err)
	}

	// Write spec files
	var filesCreated []string
	for filename, content := range generatedFiles {
		filePath := filepath.Join(specTaskDir, filename)
		err = os.WriteFile(filePath, []byte(content), 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to write file %s: %w", filename, err)
		}
		filesCreated = append(filesCreated, filepath.Join("specs", specTask.ID, filename))
	}

	// Stage files for commit
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	for _, file := range filesCreated {
		_, err = worktree.Add(file)
		if err != nil {
			return nil, fmt.Errorf("failed to stage file %s: %w", file, err)
		}
	}

	// Create commit
	commitMessage := config.CommitMessage
	if commitMessage == "" {
		commitMessage = fmt.Sprintf("Add specifications for %s\n\nGenerated from SpecTask %s\nIncludes requirements, design, and implementation tasks\n\nApproved by: %s",
			specTask.Name,
			specTask.ID,
			specTask.SpecApprovedBy,
		)
	}

	commit, err := worktree.Commit(commitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  s.gitUserName,
			Email: s.gitUserEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create commit: %w", err)
	}

	result := &SpecDocumentResult{
		SpecTaskID:   specTask.ID,
		CommitHash:   commit.String(),
		BranchName:   branchName,
		FilesCreated: filesCreated,
		Success:      true,
		Message:      "Spec documents committed to git successfully",
	}

	// TODO: Create pull request if requested
	if config.CreatePullRequest {
		// Implementation would create a pull request
		result.PullRequestURL = fmt.Sprintf("https://github.com/project/repo/pull/%s", branchName)
		result.Warnings = append(result.Warnings, "Pull request creation not yet implemented")
	}

	return result, nil
}

// Helper methods

func (s *SpecDocumentService) convertToEARSNotation(requirementsSpec string) string {
	var content strings.Builder

	// This is a simplified conversion - in production this would be more sophisticated
	// Split requirements into sections and convert to EARS format

	content.WriteString("### Functional Requirements\n\n")

	// Simple pattern matching to convert requirements to EARS notation
	lines := strings.Split(requirementsSpec, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.Contains(line, "can") || strings.Contains(line, "should") || strings.Contains(line, "must") {
			// Convert to EARS format
			earsFormat := s.convertLineToEARS(line)
			content.WriteString(earsFormat)
			content.WriteString("\n\n")
		}
	}

	return content.String()
}

func (s *SpecDocumentService) convertLineToEARS(requirement string) string {
	// Simple conversion to EARS notation
	// This is a basic implementation - production would be more sophisticated

	// Example: "Users can register with email" -> "WHEN a user provides valid email and password\nTHE SYSTEM SHALL create a new user account"

	if strings.Contains(requirement, "register") {
		return "WHEN a user provides valid registration information\nTHE SYSTEM SHALL create a new user account and send verification email"
	} else if strings.Contains(requirement, "login") {
		return "WHEN a user provides valid credentials\nTHE SYSTEM SHALL authenticate the user and create a session"
	} else if strings.Contains(requirement, "logout") {
		return "WHEN a user requests logout\nTHE SYSTEM SHALL invalidate the session and redirect to login page"
	} else {
		// Generic conversion
		return fmt.Sprintf("WHEN the appropriate conditions are met\nTHE SYSTEM SHALL %s", strings.ToLower(requirement))
	}
}

func (s *SpecDocumentService) extractUserStoryFromTask(task *types.SpecTaskImplementationTask) string {
	// Extract user story from implementation task
	// This is simplified - production would have more sophisticated extraction
	return fmt.Sprintf("As a user, I want %s", strings.ToLower(task.Title))
}

func (s *SpecDocumentService) generateTaskBoard(tasks []*types.SpecTaskImplementationTask) string {
	var content strings.Builder

	content.WriteString("## Task Board\n\n")
	content.WriteString("| Task | Status | Effort | Dependencies | Assigned Session |\n")
	content.WriteString("|------|--------|--------|--------------|------------------|\n")

	for i, task := range tasks {
		deps := "None"
		if task.HasDependencies() {
			var depIndices []int
			if err := json.Unmarshal(task.Dependencies, &depIndices); err == nil {
				depNames := make([]string, len(depIndices))
				for j, depIndex := range depIndices {
					depNames[j] = fmt.Sprintf("Task %d", depIndex+1)
				}
				deps = strings.Join(depNames, ", ")
			}
		}

		assignedSession := task.AssignedWorkSessionID
		if assignedSession == "" {
			assignedSession = "TBD"
		}

		content.WriteString(fmt.Sprintf("| %d. %s | %s | %s | %s | %s |\n",
			i+1,
			task.Title,
			string(task.Status),
			task.EstimatedEffort,
			deps,
			assignedSession,
		))
	}

	content.WriteString("\n")
	return content.String()
}

func (s *SpecDocumentService) getRepositoryPath(projectPath string, specTask *types.SpecTask) string {
	if projectPath != "" {
		return projectPath
	}

	// Default repository path based on SpecTask
	return filepath.Join(s.gitBasePath, specTask.ProjectID, specTask.ID)
}

func (s *SpecDocumentService) openOrCreateRepository(repoPath string) (*git.Repository, error) {
	// Try to open existing repository
	repo, err := git.PlainOpen(repoPath)
	if err == nil {
		return repo, nil
	}

	// Create new repository if it doesn't exist
	err = os.MkdirAll(repoPath, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository directory: %w", err)
	}

	repo, err = git.PlainInit(repoPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize repository: %w", err)
	}

	log.Info().Str("repo_path", repoPath).Msg("Created new git repository for spec documents")
	return repo, nil
}

func (s *SpecDocumentService) createOrCheckoutBranch(repo *git.Repository, branchName string) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Try to checkout existing branch
	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
	})
	if err == nil {
		return nil // Branch exists and checked out
	}

	// Create new branch
	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
		Create: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}

	log.Info().Str("branch_name", branchName).Msg("Created new branch for spec documents")
	return nil
}

// Public API methods for integration

// CommitImplementationProgress commits updates to tasks.md with current progress
func (s *SpecDocumentService) CommitImplementationProgress(
	ctx context.Context,
	specTaskID string,
	progressSummary string,
) error {
	if s.testMode {
		log.Info().Str("spec_task_id", specTaskID).Msg("Test mode: skipping progress commit")
		return nil
	}

	specTask, err := s.store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		return fmt.Errorf("failed to get SpecTask: %w", err)
	}

	// Get current progress
	progress, err := s.store.GetSpecTaskProgress(ctx, specTaskID)
	if err != nil {
		return fmt.Errorf("failed to get progress: %w", err)
	}

	// Update tasks.md with current progress
	config := &SpecDocumentConfig{
		SpecTaskID:        specTaskID,
		ProjectPath:       specTask.ProjectPath,
		CommitMessage:     fmt.Sprintf("Update implementation progress: %s\n\nProgress: %.1f%% complete\n%s", specTask.Name, progress.OverallProgress*100, progressSummary),
		IncludeTimestamps: true,
		GenerateTaskBoard: true,
	}

	// Regenerate tasks.md with updated progress
	tasksContent := s.generateTasksMarkdown(specTask, config)

	// Commit updated tasks.md
	return s.commitSingleFile(ctx, config, "tasks.md", tasksContent, "update_progress")
}

// CommitSessionHistory commits session history and coordination events
func (s *SpecDocumentService) CommitSessionHistory(
	ctx context.Context,
	specTaskID string,
	sessionHistory map[string]interface{},
) error {
	if s.testMode {
		log.Info().Str("spec_task_id", specTaskID).Msg("Test mode: skipping session history commit")
		return nil
	}

	// Generate session history markdown
	historyContent := s.generateSessionHistoryMarkdown(sessionHistory, specTaskID)

	// Commit to git
	config := &SpecDocumentConfig{
		SpecTaskID:    specTaskID,
		CommitMessage: fmt.Sprintf("Add session history for %s", specTaskID),
	}

	return s.commitSingleFile(ctx, config, "session-history.md", historyContent, "add_history")
}

func (s *SpecDocumentService) commitSingleFile(
	ctx context.Context,
	config *SpecDocumentConfig,
	filename string,
	content string,
	operation string,
) error {
	specTask, err := s.store.GetSpecTask(ctx, config.SpecTaskID)
	if err != nil {
		return fmt.Errorf("failed to get SpecTask: %w", err)
	}

	repoPath := s.getRepositoryPath(config.ProjectPath, specTask)
	repo, err := s.openOrCreateRepository(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Write file
	specTaskDir := filepath.Join(repoPath, "specs", specTask.ID)
	filePath := filepath.Join(specTaskDir, filename)

	err = os.MkdirAll(specTaskDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	err = os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Stage and commit
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	relativePath := filepath.Join("specs", specTask.ID, filename)
	_, err = worktree.Add(relativePath)
	if err != nil {
		return fmt.Errorf("failed to stage file: %w", err)
	}

	_, err = worktree.Commit(config.CommitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  s.gitUserName,
			Email: s.gitUserEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit file: %w", err)
	}

	log.Info().
		Str("spec_task_id", config.SpecTaskID).
		Str("filename", filename).
		Str("operation", operation).
		Msg("Successfully committed file to git")

	return nil
}

func (s *SpecDocumentService) generateSessionHistoryMarkdown(
	sessionHistory map[string]interface{},
	specTaskID string,
) string {
	var content strings.Builder

	content.WriteString(fmt.Sprintf(`# Session History: %s

> **Generated**: %s
> **SpecTask**: %s

## Session Timeline

`,
		specTaskID,
		time.Now().Format("2006-01-02 15:04:05"),
		specTaskID,
	))

	// Add session history content
	if sessions, ok := sessionHistory["sessions"].([]interface{}); ok {
		for _, sessionData := range sessions {
			if session, ok := sessionData.(map[string]interface{}); ok {
				sessionName, _ := session["name"].(string)
				status, _ := session["status"].(string)
				startTime, _ := session["started_at"].(string)

				content.WriteString(fmt.Sprintf("### %s\n", sessionName))
				content.WriteString(fmt.Sprintf("- **Status**: %s\n", status))
				content.WriteString(fmt.Sprintf("- **Started**: %s\n", startTime))

				if activities, ok := session["activities"].([]interface{}); ok {
					content.WriteString("- **Activities**:\n")
					for _, activity := range activities {
						if actStr, ok := activity.(string); ok {
							content.WriteString(fmt.Sprintf("  - %s\n", actStr))
						}
					}
				}
				content.WriteString("\n")
			}
		}
	}

	return content.String()
}

// Utility function to generate task name from prompt
func generateSpecTaskName(prompt string) string {
	// Simple task name generation
	words := strings.Fields(prompt)
	if len(words) > 10 {
		return strings.Join(words[:10], " ") + "..."
	}
	return prompt
}
