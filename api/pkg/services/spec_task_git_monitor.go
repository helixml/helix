package services

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SpecTaskGitMonitor monitors Git repositories for design doc pushes
// Automatically creates design reviews when design docs are pushed
type SpecTaskGitMonitor struct {
	store          store.Store
	gitRepoService *GitRepositoryService
	pollInterval   time.Duration
	stopChan       chan struct{}
	designDocPaths []string // Paths to monitor for design docs
}

// NewSpecTaskGitMonitor creates a new Git monitor for spec tasks
func NewSpecTaskGitMonitor(
	store store.Store,
	gitRepoService *GitRepositoryService,
	pollInterval time.Duration,
) *SpecTaskGitMonitor {
	if pollInterval == 0 {
		pollInterval = 30 * time.Second // Default poll every 30 seconds
	}

	return &SpecTaskGitMonitor{
		store:          store,
		gitRepoService: gitRepoService,
		pollInterval:   pollInterval,
		stopChan:       make(chan struct{}),
		designDocPaths: []string{
			"design/",
			"docs/design/",
			".helix/design/",
		},
	}
}

// Start begins monitoring Git repositories for changes
func (m *SpecTaskGitMonitor) Start(ctx context.Context) error {
	log.Info().Msg("[GitMonitor] Starting Git push monitor for spec tasks")

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("[GitMonitor] Stopping Git push monitor")
			return nil
		case <-m.stopChan:
			log.Info().Msg("[GitMonitor] Stopping Git push monitor (stop signal)")
			return nil
		case <-ticker.C:
			if err := m.checkForUpdates(ctx); err != nil {
				log.Error().Err(err).Msg("[GitMonitor] Error checking for updates")
			}
		}
	}
}

// Stop stops the Git monitor
func (m *SpecTaskGitMonitor) Stop() {
	close(m.stopChan)
}

// checkForUpdates checks all spec task repositories for new pushes
func (m *SpecTaskGitMonitor) checkForUpdates(ctx context.Context) error {
	// Get all spec tasks in planning phase
	specTasks, err := m.store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		Status: types.TaskStatusSpecGeneration,
		Limit:  1000, // Check up to 1000 active spec tasks
	})
	if err != nil {
		return fmt.Errorf("failed to list spec tasks: %w", err)
	}

	log.Debug().Int("count", len(specTasks)).Msg("[GitMonitor] Checking spec tasks for updates")

	for _, specTask := range specTasks {
		if err := m.checkSpecTaskRepository(ctx, specTask); err != nil {
			log.Error().
				Err(err).
				Str("spec_task_id", specTask.ID).
				Msg("[GitMonitor] Error checking spec task repository")
			// Continue checking other tasks
		}
	}

	return nil
}

// checkSpecTaskRepository checks a single spec task's repository for new commits
func (m *SpecTaskGitMonitor) checkSpecTaskRepository(ctx context.Context, specTask *types.SpecTask) error {
	// Get project to find repository
	project, err := m.store.GetProject(ctx, specTask.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	if project.DefaultRepoID == "" {
		// No repository configured, skip
		return nil
	}

	// Get repository
	repo, err := m.store.GetGitRepository(ctx, project.DefaultRepoID)
	if err != nil {
		return fmt.Errorf("failed to get repository: %w", err)
	}

	// Open git repository
	gitRepo, err := git.PlainOpen(repo.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get the HEAD reference
	headRef, err := gitRepo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD reference: %w", err)
	}

	// Check if we've already processed this commit
	existingEvent, err := m.store.GetSpecTaskGitPushEventByCommit(ctx, specTask.ID, headRef.Hash().String())
	if err == nil && existingEvent != nil {
		// Already processed this commit
		return nil
	}

	// Get the commit
	commit, err := gitRepo.CommitObject(headRef.Hash())
	if err != nil {
		return fmt.Errorf("failed to get commit object: %w", err)
	}

	// Check if this commit contains design doc changes
	designDocsChanged, files, err := m.hasDesignDocChanges(gitRepo, commit)
	if err != nil {
		return fmt.Errorf("failed to check for design doc changes: %w", err)
	}

	if !designDocsChanged {
		// No design doc changes in this commit
		return nil
	}

	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("commit", headRef.Hash().String()).
		Strs("files", files).
		Msg("[GitMonitor] Design docs pushed, creating review")

	// Create Git push event
	filesJSON, _ := json.Marshal(files)
	pushEvent := &types.SpecTaskGitPushEvent{
		SpecTaskID:    specTask.ID,
		CommitHash:    headRef.Hash().String(),
		Branch:        headRef.Name().Short(),
		AuthorName:    commit.Author.Name,
		AuthorEmail:   commit.Author.Email,
		CommitMessage: commit.Message,
		PushedAt:      commit.Author.When,
		Processed:     false,
		FilesChanged:  filesJSON,
		EventSource:   "polling",
	}

	if err := m.store.CreateSpecTaskGitPushEvent(ctx, pushEvent); err != nil {
		return fmt.Errorf("failed to create git push event: %w", err)
	}

	// Process the event immediately
	if err := m.processGitPushEvent(ctx, pushEvent, specTask, repo.LocalPath); err != nil {
		// Mark as failed
		pushEvent.ProcessingError = err.Error()
		if updateErr := m.store.UpdateSpecTaskGitPushEvent(ctx, pushEvent); updateErr != nil {
			log.Error().Err(updateErr).Msg("[GitMonitor] Failed to update push event with error")
		}
		return fmt.Errorf("failed to process git push event: %w", err)
	}

	// Mark as processed
	now := time.Now()
	pushEvent.Processed = true
	pushEvent.ProcessedAt = &now
	if err := m.store.UpdateSpecTaskGitPushEvent(ctx, pushEvent); err != nil {
		return fmt.Errorf("failed to update git push event: %w", err)
	}

	return nil
}

// hasDesignDocChanges checks if a commit contains changes to design documents
func (m *SpecTaskGitMonitor) hasDesignDocChanges(repo *git.Repository, commit *object.Commit) (bool, []string, error) {
	var changedFiles []string

	// Get parent commit for diff
	var parentTree *object.Tree
	if commit.NumParents() > 0 {
		parent, err := commit.Parent(0)
		if err != nil {
			return false, nil, fmt.Errorf("failed to get parent commit: %w", err)
		}
		parentTree, err = parent.Tree()
		if err != nil {
			return false, nil, fmt.Errorf("failed to get parent tree: %w", err)
		}
	}

	// Get current commit tree
	tree, err := commit.Tree()
	if err != nil {
		return false, nil, fmt.Errorf("failed to get commit tree: %w", err)
	}

	// Get changes
	var changes object.Changes
	if parentTree != nil {
		changes, err = parentTree.Diff(tree)
		if err != nil {
			return false, nil, fmt.Errorf("failed to get diff: %w", err)
		}
	} else {
		// First commit - all files are new
		err = tree.Files().ForEach(func(f *object.File) error {
			changedFiles = append(changedFiles, f.Name)
			return nil
		})
		if err != nil {
			return false, nil, err
		}
		return m.containsDesignDocs(changedFiles), changedFiles, nil
	}

	// Extract file paths from changes
	for _, change := range changes {
		from, to, err := change.Files()
		if err != nil {
			continue
		}
		if from != nil {
			changedFiles = append(changedFiles, from.Name)
		}
		if to != nil {
			changedFiles = append(changedFiles, to.Name)
		}
	}

	return m.containsDesignDocs(changedFiles), changedFiles, nil
}

// containsDesignDocs checks if any of the files are design documents
func (m *SpecTaskGitMonitor) containsDesignDocs(files []string) bool {
	for _, file := range files {
		// Normalize path separators
		file = filepath.ToSlash(file)

		// Check if file is in design doc directories
		for _, designPath := range m.designDocPaths {
			if strings.HasPrefix(file, designPath) {
				// Check if it's a markdown file
				if strings.HasSuffix(strings.ToLower(file), ".md") {
					return true
				}
			}
		}
	}
	return false
}

// readSpecDocsFromGit reads the spec documents from helix-specs branch
// Returns requirements, design, and implementation plan content
// Falls back to empty strings if files cannot be read (caller should handle gracefully)
func (m *SpecTaskGitMonitor) readSpecDocsFromGit(repoPath string, specTaskID string) (requirementsSpec, technicalDesign, implementationPlan string) {
	// Find task directory in helix-specs branch
	taskDir, err := findTaskDirectory(repoPath, specTaskID)
	if err != nil {
		log.Debug().Err(err).Str("spec_task_id", specTaskID).Msg("[GitMonitor] Could not find task directory in helix-specs")
		return "", "", ""
	}

	// Read each spec document from git
	docMap := map[string]*string{
		"requirements.md": &requirementsSpec,
		"design.md":       &technicalDesign,
		"tasks.md":        &implementationPlan,
	}

	for filename, contentPtr := range docMap {
		filePath := fmt.Sprintf("%s/%s", taskDir, filename)
		content, err := readFileFromBranch(repoPath, "helix-specs", filePath)
		if err != nil {
			log.Debug().
				Err(err).
				Str("filename", filename).
				Str("path", filePath).
				Msg("[GitMonitor] Could not read spec doc from helix-specs")
			continue
		}
		*contentPtr = content
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Str("task_dir", taskDir).
		Int("requirements_len", len(requirementsSpec)).
		Int("design_len", len(technicalDesign)).
		Int("plan_len", len(implementationPlan)).
		Msg("[GitMonitor] Read spec docs from helix-specs branch")

	return requirementsSpec, technicalDesign, implementationPlan
}

// processGitPushEvent processes a Git push event and creates a design review
func (m *SpecTaskGitMonitor) processGitPushEvent(ctx context.Context, event *types.SpecTaskGitPushEvent, specTask *types.SpecTask, repoPath string) error {
	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("commit", event.CommitHash).
		Msg("[GitMonitor] Processing Git push event")

	// Mark any existing reviews as superseded
	existingReviews, err := m.store.ListSpecTaskDesignReviews(ctx, specTask.ID)
	if err != nil {
		log.Error().Err(err).Msg("[GitMonitor] Failed to list existing reviews")
	} else {
		for _, review := range existingReviews {
			if review.Status != types.SpecTaskDesignReviewStatusApproved &&
				review.Status != types.SpecTaskDesignReviewStatusSuperseded {
				review.Status = types.SpecTaskDesignReviewStatusSuperseded
				if updateErr := m.store.UpdateSpecTaskDesignReview(ctx, &review); updateErr != nil {
					log.Error().Err(updateErr).Str("review_id", review.ID).Msg("[GitMonitor] Failed to supersede review")
				}
			}
		}
	}

	// Read spec content from helix-specs branch (fresh from git, not stale database fields)
	requirementsSpec, technicalDesign, implementationPlan := m.readSpecDocsFromGit(repoPath, specTask.ID)

	// Create new design review with content from git
	review := &types.SpecTaskDesignReview{
		SpecTaskID:         specTask.ID,
		Status:             types.SpecTaskDesignReviewStatusPending,
		GitCommitHash:      event.CommitHash,
		GitBranch:          event.Branch,
		GitPushedAt:        event.PushedAt,
		RequirementsSpec:   requirementsSpec,
		TechnicalDesign:    technicalDesign,
		ImplementationPlan: implementationPlan,
	}

	if err := m.store.CreateSpecTaskDesignReview(ctx, review); err != nil {
		return fmt.Errorf("failed to create design review: %w", err)
	}

	log.Info().
		Str("review_id", review.ID).
		Str("spec_task_id", specTask.ID).
		Bool("just_do_it_mode", specTask.JustDoItMode).
		Msg("[GitMonitor] Created design review")

	// Just Do It mode skips spec generation entirely, so this code path shouldn't be hit for those tasks
	// Move to spec_review for human review
	specTask.Status = types.TaskStatusSpecReview

	log.Info().
		Str("spec_task_id", specTask.ID).
		Msg("[GitMonitor] Moved to spec review, awaiting human approval")

	if err := m.store.UpdateSpecTask(ctx, specTask); err != nil {
		return fmt.Errorf("failed to update spec task status: %w", err)
	}

	return nil
}

// HandleWebhook processes a webhook from an external Git provider (GitHub, GitLab, etc.)
func (m *SpecTaskGitMonitor) HandleWebhook(ctx context.Context, provider string, payload map[string]interface{}) error {
	log.Info().
		Str("provider", provider).
		Msg("[GitMonitor] Received Git webhook")

	// Extract commit information based on provider
	var commitHash string
	// var branch, authorName, authorEmail, message string
	// var pushedAt time.Time

	switch strings.ToLower(provider) {
	case "github":
		// GitHub webhook format (TODO: implement when webhook integration is ready)
		if headCommit, ok := payload["head_commit"].(map[string]interface{}); ok {
			if id, ok := headCommit["id"].(string); ok {
				commitHash = id
			}
		}
	case "gitlab":
		// GitLab webhook format (TODO: implement when webhook integration is ready)
		if commits, ok := payload["commits"].([]interface{}); ok && len(commits) > 0 {
			if headCommit, ok := commits[len(commits)-1].(map[string]interface{}); ok {
				if id, ok := headCommit["id"].(string); ok {
					commitHash = id
				}
			}
		}
	default:
		return fmt.Errorf("unsupported provider: %s", provider)
	}

	if commitHash == "" {
		return fmt.Errorf("no commit hash found in webhook payload")
	}

	// Find spec task associated with this repository
	// This would need to be enhanced to properly match repositories
	// For now, we'll skip webhook processing and rely on polling
	log.Warn().
		Str("provider", provider).
		Str("commit", commitHash).
		Msg("[GitMonitor] Webhook processing not yet implemented, will be picked up by polling")

	return nil
}
