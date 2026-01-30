package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

// HelixRepositoryGrepTool - searches file contents for text patterns

var grepParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"pattern": {
			Type:        jsonschema.String,
			Description: "Text pattern to search for (supports regular expressions). Examples: 'func.*Error', 'TODO:', 'class \\w+Controller'",
		},
		"path": {
			Type:        jsonschema.String,
			Description: "Directory path to search in (use '.' or empty string for root). Searches recursively in all subdirectories.",
		},
		"file_pattern": {
			Type:        jsonschema.String,
			Description: "Optional file name pattern to filter which files to search (e.g., '*.go', '*.py'). If not specified, searches all files.",
		},
		"branch": {
			Type:        jsonschema.String,
			Description: "Branch name (defaults to repository default branch if not specified)",
		},
		"case_sensitive": {
			Type:        jsonschema.Boolean,
			Description: "Whether the search should be case sensitive (default: false)",
		},
		"limit": {
			Type:        jsonschema.Number,
			Description: "Maximum number of matches to return (default: 50)",
		},
	},
	Required: []string{"pattern"},
}

type HelixRepositoryGrepTool struct {
	store                store.Store
	gitRepositoryService *services.GitRepositoryService
	projectID            string
}

func NewHelixRepositoryGrepTool(projectID string, store store.Store, gitRepositoryService *services.GitRepositoryService) *HelixRepositoryGrepTool {
	return &HelixRepositoryGrepTool{
		store:                store,
		gitRepositoryService: gitRepositoryService,
		projectID:            projectID,
	}
}

var _ agent.Tool = &HelixRepositoryGrepTool{}

func (t *HelixRepositoryGrepTool) Name() string {
	return "GrepFiles"
}

func (t *HelixRepositoryGrepTool) Description() string {
	return "Search for text patterns in file contents across the repository"
}

func (t *HelixRepositoryGrepTool) String() string {
	return "GrepFiles"
}

func (t *HelixRepositoryGrepTool) StatusMessage() string {
	return "Searching file contents"
}

func (t *HelixRepositoryGrepTool) Icon() string {
	return "SearchIcon"
}

func (t *HelixRepositoryGrepTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "GrepFiles",
				Description: "Search for text patterns in file contents across the repository. Supports regular expressions and can filter by file patterns. Returns matching lines with file paths and line numbers.",
				Parameters:  grepParameters,
			},
		},
	}
}

type GrepMatch struct {
	File       string `json:"file"`
	LineNumber int    `json:"line_number"`
	Line       string `json:"line"`
}

type GrepResult struct {
	Pattern       string      `json:"pattern"`
	Path          string      `json:"path"`
	FilePattern   string      `json:"file_pattern"`
	Branch        string      `json:"branch"`
	CaseSensitive bool        `json:"case_sensitive"`
	Matches       []GrepMatch `json:"matches"`
	Total         int         `json:"total"`
}

func (r *GrepResult) ToString() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Pattern: %s\n", r.Pattern))
	if r.FilePattern != "" {
		sb.WriteString(fmt.Sprintf("File pattern: %s\n", r.FilePattern))
	}
	sb.WriteString(fmt.Sprintf("Search path: %s\n", r.Path))
	sb.WriteString(fmt.Sprintf("Branch: %s\n", r.Branch))
	sb.WriteString(fmt.Sprintf("Case sensitive: %v\n", r.CaseSensitive))
	sb.WriteString(fmt.Sprintf("Total matches: %d\n\n", r.Total))

	if len(r.Matches) == 0 {
		sb.WriteString("No matches found.\n")
		return sb.String()
	}

	sb.WriteString("Matches:\n")
	for _, match := range r.Matches {
		sb.WriteString(fmt.Sprintf("%s:%d: %s\n", match.File, match.LineNumber, strings.TrimSpace(match.Line)))
	}

	return sb.String()
}

func (t *HelixRepositoryGrepTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
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
		Msg("Executing GrepFiles tool")

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

	filePattern := ""
	if fp, ok := args["file_pattern"].(string); ok {
		filePattern = fp
	}

	branch := repo.DefaultBranch
	if b, ok := args["branch"].(string); ok && b != "" {
		branch = b
	}

	if branch == "" {
		return "", fmt.Errorf("branch is required (repository has no default branch)")
	}

	caseSensitive := false
	if cs, ok := args["case_sensitive"].(bool); ok {
		caseSensitive = cs
	}

	limit := 50
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	// Compile regex pattern
	regexFlags := "(?m)" // Multiline mode
	if !caseSensitive {
		regexFlags += "(?i)" // Case insensitive
	}
	regex, err := regexp.Compile(regexFlags + pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex pattern: %w", err)
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

	// Filter files by search path and file pattern
	var filesToSearch []string
	for _, file := range allFiles {
		// If search path specified, filter by it
		if searchPath != "" && searchPath != "." {
			normalizedSearchPath := strings.TrimPrefix(searchPath, "./")
			normalizedSearchPath = strings.TrimSuffix(normalizedSearchPath, "/")

			if !strings.HasPrefix(file, normalizedSearchPath+"/") && file != normalizedSearchPath {
				continue
			}
		}

		// If file pattern specified, match against it
		if filePattern != "" {
			fileName := filepath.Base(file)
			matched, err := filepath.Match(filePattern, fileName)
			if err != nil {
				log.Warn().Err(err).Str("file_pattern", filePattern).Msg("Invalid file pattern")
				return "", fmt.Errorf("invalid file pattern: %w", err)
			}
			if !matched {
				continue
			}
		}

		filesToSearch = append(filesToSearch, file)
	}

	// Search for pattern in each file
	var matches []GrepMatch
	err = t.gitRepositoryService.WithExternalRepoRead(ctx, repo, func() error {
		gitRepo, err := services.OpenGitRepo(repo.LocalPath)
		if err != nil {
			return fmt.Errorf("failed to open repository: %w", err)
		}
		defer gitRepo.Close()

		for _, file := range filesToSearch {
			if len(matches) >= limit {
				break
			}

			// Read file content
			content, err := gitRepo.ReadFileFromBranch(branch, file)
			if err != nil {
				log.Debug().Err(err).Str("file", file).Msg("Failed to read file (skipping)")
				continue
			}

			// Skip binary files (heuristic: check for null bytes)
			contentStr := string(content)
			if strings.Contains(contentStr, "\x00") {
				continue
			}

			// Search for pattern in file
			lines := strings.Split(contentStr, "\n")
			for lineNum, line := range lines {
				if regex.MatchString(line) {
					matches = append(matches, GrepMatch{
						File:       file,
						LineNumber: lineNum + 1, // 1-indexed
						Line:       line,
					})

					if len(matches) >= limit {
						break
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to search files")
		return "", fmt.Errorf("failed to search files: %w", err)
	}

	result := GrepResult{
		Pattern:       pattern,
		Path:          searchPath,
		FilePattern:   filePattern,
		Branch:        branch,
		CaseSensitive: caseSensitive,
		Matches:       matches,
		Total:         len(matches),
	}

	return result.ToString(), nil
}
