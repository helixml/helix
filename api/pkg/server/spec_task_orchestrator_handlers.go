package server

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/rs/zerolog/log"
)

// @Summary Get design docs for SpecTask
// @Description Get the design documents from helix-specs worktree
// @Tags SpecTasks
// @Produce json
// @Param id path string true "SpecTask ID"
// @Success 200 {object} DesignDocsResponse
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/spec-tasks/{id}/design-docs [get]
func (apiServer *HelixAPIServer) getSpecTaskDesignDocs(_ http.ResponseWriter, req *http.Request) (*DesignDocsResponse, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	taskID := vars["id"]

	// Get task
	task, err := apiServer.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		return nil, system.NewHTTPError404("task not found")
	}

	// Get the project's default repository (design docs repo)
	project, err := apiServer.Store.GetProject(ctx, task.ProjectID)
	if err != nil {
		return nil, system.NewHTTPError500("failed to get project")
	}

	if project.DefaultRepoID == "" {
		return nil, system.NewHTTPError400("project has no default repository")
	}

	// Get repository
	repo, err := apiServer.gitRepositoryService.GetRepository(ctx, project.DefaultRepoID)
	if err != nil {
		return nil, system.NewHTTPError500("failed to get repository")
	}

	// Read design docs from Git repository helix-specs branch
	// Docs are in task-specific subdirectory: tasks/{date}_{name}_{task_id}/
	designDocs, err := apiServer.readDesignDocsFromGit(repo.LocalPath, task.ID)
	if err != nil {
		log.Error().Err(err).Str("repo_path", repo.LocalPath).Str("task_id", task.ID).Msg("Failed to read design docs from git")
		return nil, system.NewHTTPError500("failed to read design docs")
	}

	response := &DesignDocsResponse{
		TaskID:    task.ID,
		Documents: designDocs,
	}

	return response, nil
}

// readDesignDocsFromGit reads design documents from the helix-specs branch
func (apiServer *HelixAPIServer) readDesignDocsFromGit(repoPath string, taskID string) ([]DesignDocument, error) {
	// First, list all files in helix-specs branch to find task directory
	// Format: design/tasks/{date}_{name}_{task_id}/
	cmd := exec.Command("git", "ls-tree", "--name-only", "-r", "helix-specs")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		// helix-specs branch might not exist yet
		log.Debug().Err(err).Str("repo_path", repoPath).Msg("No helix-specs branch found")
		return []DesignDocument{}, nil
	}

	// Find task directory by matching task ID in any file path
	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	var taskDir string
	for _, file := range files {
		if strings.Contains(file, taskID) {
			// Extract directory path (e.g., design/tasks/2025-11-11_..._taskid/)
			parts := strings.Split(file, "/")
			if len(parts) >= 3 {
				taskDir = strings.Join(parts[:len(parts)-1], "/")
				break
			}
		}
	}

	if taskDir == "" {
		log.Debug().Str("task_id", taskID).Msg("No design docs directory found for task in helix-specs")
		return []DesignDocument{}, nil
	}

	log.Info().Str("task_dir", taskDir).Str("task_id", taskID).Msg("Found design docs directory")

	// Read all .md files from the task directory
	var docs []DesignDocument
	for _, file := range files {
		if !strings.HasPrefix(file, taskDir+"/") || !strings.HasSuffix(file, ".md") {
			continue
		}

		// Read file content from helix-specs branch
		contentCmd := exec.Command("git", "show", fmt.Sprintf("helix-specs:%s", file))
		contentCmd.Dir = repoPath
		content, err := contentCmd.CombinedOutput()
		if err != nil {
			log.Warn().Err(err).Str("file", file).Msg("Failed to read design doc file")
			continue
		}

		// Extract just the filename (e.g., requirements.md)
		filename := strings.TrimPrefix(file, taskDir+"/")

		docs = append(docs, DesignDocument{
			Filename: filename,
			Content:  string(content),
			Path:     file,
		})
	}

	log.Info().Int("doc_count", len(docs)).Str("task_id", taskID).Msg("Read design documents from helix-specs")
	return docs, nil
}

// Response types

type DesignDocsResponse struct {
	TaskID    string           `json:"task_id"`
	Documents []DesignDocument `json:"documents"`
}

type DesignDocument struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
	Path     string `json:"path"`
}
