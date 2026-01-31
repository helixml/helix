package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

// HelixRepositoryFindFilesTool - finds files matching a pattern

var findFilesParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"pattern": {
			Type:        jsonschema.String,
			Description: "File name pattern to match (supports wildcards: * matches any sequence, ? matches single character). Examples: '*.go', 'test_*.py', 'README.*'",
		},
		"path": {
			Type:        jsonschema.String,
			Description: "Directory path to search in (use '.' or empty string for root). Searches recursively in all subdirectories.",
		},
		"branch": {
			Type:        jsonschema.String,
			Description: "Branch name (defaults to repository default branch if not specified)",
		},
		"limit": {
			Type:        jsonschema.Number,
			Description: "Maximum number of files to return (default: 100)",
		},
	},
	Required: []string{"pattern"},
}

type HelixRepositoryFindFilesTool struct {
	store                store.Store
	gitRepositoryService *services.GitRepositoryService
	projectID            string
}

func NewHelixRepositoryFindFilesTool(projectID string, store store.Store, gitRepositoryService *services.GitRepositoryService) *HelixRepositoryFindFilesTool {
	return &HelixRepositoryFindFilesTool{
		store:                store,
		gitRepositoryService: gitRepositoryService,
		projectID:            projectID,
	}
}

var _ agent.Tool = &HelixRepositoryFindFilesTool{}

func (t *HelixRepositoryFindFilesTool) Name() string {
	return "FindFiles"
}

func (t *HelixRepositoryFindFilesTool) Description() string {
	return "Find files matching a pattern in the repository"
}

func (t *HelixRepositoryFindFilesTool) String() string {
	return "FindFiles"
}

func (t *HelixRepositoryFindFilesTool) StatusMessage() string {
	return "Finding files in repository"
}

func (t *HelixRepositoryFindFilesTool) Icon() string {
	return "SearchIcon"
}

func (t *HelixRepositoryFindFilesTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "FindFiles",
				Description: "Find files matching a pattern in the repository. Supports wildcards (* and ?) and searches recursively in all subdirectories. Use this to locate files by name pattern.",
				Parameters:  findFilesParameters,
			},
		},
	}
}

type FindFilesResult struct {
	Pattern string   `json:"pattern"`
	Path    string   `json:"path"`
	Branch  string   `json:"branch"`
	Files   []string `json:"files"`
	Total   int      `json:"total"`
}

func (r *FindFilesResult) ToString() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Pattern: %s\n", r.Pattern))
	sb.WriteString(fmt.Sprintf("Search path: %s\n", r.Path))
	sb.WriteString(fmt.Sprintf("Branch: %s\n", r.Branch))
	sb.WriteString(fmt.Sprintf("Total matches: %d\n\n", r.Total))

	if len(r.Files) == 0 {
		sb.WriteString("No files found matching pattern.\n")
		return sb.String()
	}

	sb.WriteString("Matching files:\n")
	for _, file := range r.Files {
		sb.WriteString(fmt.Sprintf("  %s\n", file))
	}

	return sb.String()
}

func (t *HelixRepositoryFindFilesTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
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
		Msg("Executing FindFiles tool")

	// Parse required pattern argument
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}

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

	// Parse optional arguments
	searchPath := ""
	if p, ok := args["path"].(string); ok {
		searchPath = p
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

	// Open the repository and list all files
	var allFiles []string
	err = t.gitRepositoryService.WithExternalRepoRead(ctx, repo, func() error {
		gitRepo, err := services.OpenGitRepo(repo.LocalPath)
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}
		defer gitRepo.Close()

		allFiles, err = gitRepo.ListFilesInBranch(branch)
		return err
	})
	if err != nil {
		log.Error().Err(err).
			Str("repo_id", repo.ID).
			Str("branch", branch).
			Msg("Failed to list files in repository")
		return "", fmt.Errorf("failed to list files: %w", err)
	}

	// Filter files by pattern and search path
	var matchingFiles []string
	for _, file := range allFiles {
		// If search path specified, filter by it
		if searchPath != "" && searchPath != "." {
			// Normalize search path
			normalizedSearchPath := strings.TrimPrefix(searchPath, "./")
			normalizedSearchPath = strings.TrimSuffix(normalizedSearchPath, "/")

			// Check if file is in the search path
			if !strings.HasPrefix(file, normalizedSearchPath+"/") && file != normalizedSearchPath {
				continue
			}
		}

		// Match pattern against file name (not full path)
		fileName := filepath.Base(file)
		matched, err := filepath.Match(pattern, fileName)
		if err != nil {
			log.Warn().Err(err).Str("pattern", pattern).Msg("Invalid pattern")
			return "", fmt.Errorf("invalid pattern: %w", err)
		}

		if matched {
			matchingFiles = append(matchingFiles, file)
			if len(matchingFiles) >= limit {
				break
			}
		}
	}

	result := FindFilesResult{
		Pattern: pattern,
		Path:    searchPath,
		Branch:  branch,
		Files:   matchingFiles,
		Total:   len(matchingFiles),
	}

	return result.ToString(), nil
}
