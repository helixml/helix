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

// HelixRepositoryGetFileTool - reads the full contents of a file

var getFileParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"path": {
			Type:        jsonschema.String,
			Description: "File path to read (e.g., 'src/main.go', 'README.md')",
		},
		"branch": {
			Type:        jsonschema.String,
			Description: "Branch name (defaults to repository default branch if not specified)",
		},
	},
	Required: []string{"path"},
}

type HelixRepositoryGetFileTool struct {
	store                store.Store
	gitRepositoryService *services.GitRepositoryService
	projectID            string
}

func NewHelixRepositoryGetFileTool(projectID string, store store.Store, gitRepositoryService *services.GitRepositoryService) *HelixRepositoryGetFileTool {
	return &HelixRepositoryGetFileTool{
		store:                store,
		gitRepositoryService: gitRepositoryService,
		projectID:            projectID,
	}
}

var _ agent.Tool = &HelixRepositoryGetFileTool{}

func (t *HelixRepositoryGetFileTool) Name() string {
	return "GetFile"
}

func (t *HelixRepositoryGetFileTool) Description() string {
	return "Read the full contents of a file from the repository"
}

func (t *HelixRepositoryGetFileTool) String() string {
	return "GetFile"
}

func (t *HelixRepositoryGetFileTool) StatusMessage() string {
	return "Reading file from repository"
}

func (t *HelixRepositoryGetFileTool) Icon() string {
	return "FileIcon"
}

func (t *HelixRepositoryGetFileTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "GetFile",
				Description: "Read the full contents of a file from the repository. Use this to examine source code, configuration files, documentation, or any other file in the repository.",
				Parameters:  getFileParameters,
			},
		},
	}
}

type GetFileResult struct {
	Path    string `json:"path"`
	Branch  string `json:"branch"`
	Content string `json:"content"`
	Size    int    `json:"size"`
}

func (r *GetFileResult) ToString() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("File: %s\n", r.Path))
	sb.WriteString(fmt.Sprintf("Branch: %s\n", r.Branch))
	sb.WriteString(fmt.Sprintf("Size: %s\n", formatSize(int64(r.Size))))
	sb.WriteString(fmt.Sprintf("---\n%s\n", r.Content))
	return sb.String()
}

func (t *HelixRepositoryGetFileTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
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
		Msg("Executing GetFile tool")

	// Parse required path argument
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("path is required")
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

	// Parse optional branch argument
	branch := repo.DefaultBranch
	if b, ok := args["branch"].(string); ok && b != "" {
		branch = b
	}

	if branch == "" {
		return "", fmt.Errorf("branch is required (repository has no default branch)")
	}

	// Read file contents
	var content string
	err = t.gitRepositoryService.WithExternalRepoRead(ctx, repo, func() error {
		var err error
		content, err = t.gitRepositoryService.GetFileContents(ctx, repo.ID, path, branch)
		return err
	})
	if err != nil {
		log.Error().Err(err).
			Str("repo_id", repo.ID).
			Str("path", path).
			Str("branch", branch).
			Msg("Failed to read file")
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	result := GetFileResult{
		Path:    path,
		Branch:  branch,
		Content: content,
		Size:    len(content),
	}

	return result.ToString(), nil
}
