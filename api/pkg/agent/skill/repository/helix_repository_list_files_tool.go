package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

// HelixRepositoryListFilesTool - lists files and directories in a repository path

var listFilesParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"path": {
			Type:        jsonschema.String,
			Description: "Directory path to list (use '.' or empty string for root)",
		},
		"branch": {
			Type:        jsonschema.String,
			Description: "Branch name (defaults to repository default branch if not specified)",
		},
		"limit": {
			Type:        jsonschema.Number,
			Description: "Maximum number of entries to return (default: 100)",
		},
	},
	Required: []string{},
}

type HelixRepositoryListFilesTool struct {
	store                store.Store
	gitRepositoryService *services.GitRepositoryService
	projectID            string
}

func NewHelixRepositoryListFilesTool(projectID string, store store.Store, gitRepositoryService *services.GitRepositoryService) *HelixRepositoryListFilesTool {
	return &HelixRepositoryListFilesTool{
		store:                store,
		gitRepositoryService: gitRepositoryService,
		projectID:            projectID,
	}
}

var _ agent.Tool = &HelixRepositoryListFilesTool{}

func (t *HelixRepositoryListFilesTool) Name() string {
	return "ListFiles"
}

func (t *HelixRepositoryListFilesTool) Description() string {
	return "List files and directories at a specific path in the repository"
}

func (t *HelixRepositoryListFilesTool) String() string {
	return "ListFiles"
}

func (t *HelixRepositoryListFilesTool) StatusMessage() string {
	return "Listing repository files"
}

func (t *HelixRepositoryListFilesTool) Icon() string {
	return "FolderIcon"
}

func (t *HelixRepositoryListFilesTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "ListFiles",
				Description: "List files and directories at a specific path in the repository. Returns file names, sizes, and whether each entry is a directory.",
				Parameters:  listFilesParameters,
			},
		},
	}
}

type ListFilesResult struct {
	Path    string              `json:"path"`
	Branch  string              `json:"branch"`
	Entries []types.TreeEntry   `json:"entries"`
	Total   int                 `json:"total"`
}

func (r *ListFilesResult) ToString() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Path: %s\n", r.Path))
	sb.WriteString(fmt.Sprintf("Branch: %s\n", r.Branch))
	sb.WriteString(fmt.Sprintf("Total entries: %d\n\n", r.Total))

	if len(r.Entries) == 0 {
		sb.WriteString("(empty directory)\n")
		return sb.String()
	}

	sb.WriteString("Entries:\n")
	for _, entry := range r.Entries {
		if entry.IsDir {
			sb.WriteString(fmt.Sprintf("  [DIR]  %s/\n", entry.Name))
		} else {
			size := formatSize(entry.Size)
			sb.WriteString(fmt.Sprintf("  [FILE] %s (%s)\n", entry.Name, size))
		}
	}

	return sb.String()
}

func formatSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func (t *HelixRepositoryListFilesTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	projectID := t.projectID
	if projectID == "" {
		projectContext, ok := types.GetHelixProjectContext(ctx)
		if !ok {
			return "", fmt.Errorf("helix project context not found")
		}
		projectID = projectContext.ProjectID
	}

	log.Info().
		Str("project_id", projectID).
		Str("user_id", meta.UserID).
		Str("session_id", meta.SessionID).
		Interface("args", args).
		Msg("Executing ListFiles tool")

	// Get the project
	project, err := t.store.GetProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get project: %w", err)
	}

	if project.DefaultRepoID == "" {
		return "", fmt.Errorf("project has no default repository")
	}

	// Get the default repository
	repo, err := t.store.GetGitRepository(ctx, project.DefaultRepoID)
	if err != nil {
		return "", fmt.Errorf("failed to get repository: %w", err)
	}

	// Parse arguments
	path := "."
	if p, ok := args["path"].(string); ok && p != "" {
		path = p
	}

	branch := repo.DefaultBranch
	if b, ok := args["branch"].(string); ok && b != "" {
		branch = b
	}

	if branch == "" {
		return "", fmt.Errorf("branch is required (repository has no default branch)")
	}

	limit := 100
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	// Execute the browse tree operation
	var entries []types.TreeEntry
	err = t.gitRepositoryService.WithExternalRepoRead(ctx, repo, func() error {
		var err error
		entries, err = t.gitRepositoryService.BrowseTree(ctx, repo.ID, path, branch)
		return err
	})
	if err != nil {
		log.Error().Err(err).
			Str("repo_id", repo.ID).
			Str("path", path).
			Str("branch", branch).
			Msg("Failed to list files")
		return "", fmt.Errorf("failed to list files: %w", err)
	}

	// Apply limit
	if len(entries) > limit {
		entries = entries[:limit]
	}

	result := ListFilesResult{
		Path:    path,
		Branch:  branch,
		Entries: entries,
		Total:   len(entries),
	}

	return result.ToString(), nil
}
