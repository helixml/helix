package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/goose"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ProjectGooseRecipe is the API view of a Goose recipe available to a
// project — the slash-command name the agent will fire, the recipe's title
// / description for UI rendering, and the list of declared parameters so
// the spec-task form can render a dynamic input per parameter. We do NOT
// expose the on-disk path here: the path is meaningful only inside the
// sandbox container and would leak hosting layout to the browser.
type ProjectGooseRecipe struct {
	Name        string                  `json:"name"`
	Title       string                  `json:"title,omitempty"`
	Description string                  `json:"description,omitempty"`
	Parameters  []goose.RecipeParameter `json:"parameters,omitempty"`
	// Error, when non-empty, indicates that the recipe was declared on the
	// agent but couldn't be loaded — repo not cloned yet, file missing,
	// YAML malformed, etc. The UI surfaces this so the user can fix the
	// project YAML before creating a task that would silently fall back to
	// vanilla goose.
	Error string `json:"error,omitempty"`
}

// listProjectGooseRecipes godoc
// @Summary List Goose recipes available to a project
// @Description Returns the parsed Goose recipes declared on the project's
// @Description default agent, including each recipe's parameter schema so
// @Description the spec-task creation form can render dynamic inputs.
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {array} ProjectGooseRecipe
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/goose-recipes [get]
func (s *HelixAPIServer) listProjectGooseRecipes(_ http.ResponseWriter, r *http.Request) ([]ProjectGooseRecipe, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	projectID := getID(r)

	project, err := s.Store.GetProject(ctx, projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}
	if err := s.authorizeUserToProject(ctx, user, project, types.ActionGet); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	if project.DefaultHelixAppID == "" {
		return []ProjectGooseRecipe{}, nil
	}
	app, err := s.Store.GetApp(ctx, project.DefaultHelixAppID)
	if err != nil {
		return nil, system.NewHTTPError404("project agent not found")
	}

	var assistant *types.AssistantConfig
	for i := range app.Config.Helix.Assistants {
		if app.Config.Helix.Assistants[i].AgentType == types.AgentTypeZedExternal {
			assistant = &app.Config.Helix.Assistants[i]
			break
		}
	}
	if assistant == nil || len(assistant.GooseRecipes) == 0 {
		return []ProjectGooseRecipe{}, nil
	}

	rootDir, repoErr := s.resolveGooseRecipeRoot(ctx, app, assistant)
	if repoErr != "" {
		// Surface the repo-level failure on every recipe entry so the UI
		// can render a clear "recipes unavailable: <reason>" rather than
		// hiding the recipes silently.
		out := make([]ProjectGooseRecipe, 0, len(assistant.GooseRecipes))
		for _, r := range assistant.GooseRecipes {
			out = append(out, ProjectGooseRecipe{Name: r.Name, Error: repoErr})
		}
		return out, nil
	}

	out := make([]ProjectGooseRecipe, 0, len(assistant.GooseRecipes))
	for _, r := range assistant.GooseRecipes {
		entry := ProjectGooseRecipe{Name: r.Name}
		clean := filepath.Clean(r.Path)
		if strings.HasPrefix(clean, "..") || strings.HasPrefix(clean, "/") {
			entry.Error = "recipe path is outside the repo"
			out = append(out, entry)
			continue
		}
		abs := filepath.Join(rootDir, clean)
		content, err := os.ReadFile(abs)
		if err != nil {
			log.Warn().Err(err).Str("recipe", r.Name).Str("path", abs).Msg("goose recipe file missing on disk")
			entry.Error = "recipe file not found in repo"
			out = append(out, entry)
			continue
		}
		recipe, err := goose.Parse(content)
		if err != nil {
			entry.Error = err.Error()
			out = append(out, entry)
			continue
		}
		entry.Title = recipe.Title
		entry.Description = recipe.Description
		entry.Parameters = recipe.Parameters
		out = append(out, entry)
	}
	return out, nil
}

// GooseRecipeCandidate is one YAML file found under the recipe-repo root
// that the user could plausibly use as a recipe. The frontend renders
// these as picker entries — `Path` is what gets persisted onto the
// agent's GooseRecipes, `Name` is the derived slash-command name shown
// next to the path, and `Title` (best-effort) is shown for context.
type GooseRecipeCandidate struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Title string `json:"title,omitempty"`
}

// GooseRecipeCandidatesResponse wraps the file list. The Repositories
// list lets the frontend render the "recipe repository" dropdown without
// a second roundtrip — it's every git repo in the agent's organization
// (recipe repos are org-scoped, not project-scoped).
type GooseRecipeCandidatesResponse struct {
	Files     []GooseRecipeCandidate `json:"files"`
	Truncated bool                   `json:"truncated,omitempty"`
	Error     string                 `json:"error,omitempty"`

	// Repositories the agent's org can use as a recipe source.
	Repositories []GooseRecipeRepoOption `json:"repositories"`
	// CurrentRepoURL is the external_url of the repo whose files we
	// walked. Mirrors assistant.GooseRecipeRepoURL — present so the UI
	// can pre-select the right dropdown entry.
	CurrentRepoURL string `json:"current_repo_url,omitempty"`
}

// GooseRecipeRepoOption is one entry in the recipe-repo dropdown.
// URL is the value persisted onto GooseRecipeRepoURL.
type GooseRecipeRepoOption struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

// resolveGooseRecipeRoot returns the absolute container-side directory
// containing the agent's recipe files. Returns ("", "human reason") when
// the repo can't be located so the API can render a friendly UI message.
//
// Recipe repos are org-scoped — they don't need to be attached to any
// particular project. The agent's GooseRecipeRepoURL must be a repo in
// the same organization as the app.
func (s *HelixAPIServer) resolveGooseRecipeRoot(ctx context.Context, app *types.App, assistant *types.AssistantConfig) (string, string) {
	if assistant.GooseRecipeRepoURL == "" {
		return "", "no recipe repository selected for this agent"
	}
	repo, err := s.Store.GetGitRepositoryByURL(ctx, app.OrganizationID, assistant.GooseRecipeRepoURL)
	if err != nil {
		return "", fmt.Sprintf("recipe repo %s not found in this organization", assistant.GooseRecipeRepoURL)
	}
	if repo.LocalPath == "" {
		return "", "recipe repository has not been cloned yet"
	}
	return repo.LocalPath, ""
}

// listAppGooseRecipeCandidates godoc
// @Summary List YAML files in an app's recipe repository (by app id)
// @Description Returns every git repo in the agent's organization as a
// @Description dropdown option, plus the walked files of whichever repo
// @Description is currently selected via assistant.GooseRecipeRepoURL.
// @Description Recipe repos are org-scoped — they don't need to be
// @Description attached to any particular project.
// @Tags Apps
// @Accept json
// @Produce json
// @Param id path string true "App ID"
// @Success 200 {object} GooseRecipeCandidatesResponse
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/apps/{id}/goose-recipes/candidates [get]
func (s *HelixAPIServer) listAppGooseRecipeCandidates(_ http.ResponseWriter, r *http.Request) (*GooseRecipeCandidatesResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	// Apps router uses {app_id} in its path pattern, not {id}.
	appID := mux.Vars(r)["app_id"]
	if appID == "" {
		appID = getID(r)
	}

	app, err := s.Store.GetApp(ctx, appID)
	if err != nil {
		return nil, system.NewHTTPError404("app not found")
	}
	// Authorize via the app itself — same access semantics as editing the app.
	if err := s.authorizeUserToApp(ctx, user, app, types.ActionGet); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	var assistant *types.AssistantConfig
	for i := range app.Config.Helix.Assistants {
		if app.Config.Helix.Assistants[i].AgentType == types.AgentTypeZedExternal {
			assistant = &app.Config.Helix.Assistants[i]
			break
		}
	}
	if assistant == nil {
		return &GooseRecipeCandidatesResponse{Files: []GooseRecipeCandidate{}, Repositories: []GooseRecipeRepoOption{}}, nil
	}

	// All git repos in the agent's organization are eligible as recipe
	// sources — recipe repos are not project-scoped.
	repos, err := s.Store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{OrganizationID: app.OrganizationID})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("list org repos: %v", err))
	}
	options := make([]GooseRecipeRepoOption, 0, len(repos))
	for _, repo := range repos {
		url := repo.ExternalURL
		if !repo.IsExternal || url == "" {
			url = repo.CloneURL
		}
		options = append(options, GooseRecipeRepoOption{URL: url, Name: repo.Name})
	}

	currentURL := assistant.GooseRecipeRepoURL

	// No repo selected yet → return the dropdown options with an empty
	// file list so the UI can prompt the user to pick one.
	if currentURL == "" {
		return &GooseRecipeCandidatesResponse{
			Files:        []GooseRecipeCandidate{},
			Repositories: options,
		}, nil
	}

	rootDir, repoErr := s.resolveGooseRecipeRoot(ctx, app, assistant)
	if repoErr != "" {
		return &GooseRecipeCandidatesResponse{
			Files:          []GooseRecipeCandidate{},
			Error:          repoErr,
			Repositories:   options,
			CurrentRepoURL: currentURL,
		}, nil
	}
	walked, walkErr := walkRecipeCandidates(rootDir)
	if walkErr != nil {
		return nil, walkErr
	}
	walked.Repositories = options
	walked.CurrentRepoURL = currentURL
	return walked, nil
}

// walkRecipeCandidates is the on-disk walk used by the candidates
// handler — separated so the walker stays testable independently of the
// repo-lookup logic.
func walkRecipeCandidates(rootDir string) (*GooseRecipeCandidatesResponse, *system.HTTPError) {
	const maxEntries = 200
	skipDirs := map[string]bool{
		"node_modules": true,
		"vendor":       true,
		".git":         true,
	}

	files := make([]GooseRecipeCandidate, 0, 32)
	truncated := false
	walkErr := filepath.Walk(rootDir, func(absPath string, info os.FileInfo, err error) error {
		if err != nil {
			log.Warn().Err(err).Str("path", absPath).Msg("goose recipe candidate walk: skipping unreadable entry")
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(info.Name())
		if !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml") {
			return nil
		}
		if len(files) >= maxEntries {
			truncated = true
			return filepath.SkipAll
		}
		rel, relErr := filepath.Rel(rootDir, absPath)
		if relErr != nil {
			return nil
		}
		entry := GooseRecipeCandidate{
			Path: filepath.ToSlash(rel),
			Name: goose.DefaultName(rel),
		}
		if info.Size() < 64*1024 {
			content, readErr := os.ReadFile(absPath)
			if readErr == nil {
				if recipe, parseErr := goose.Parse(content); parseErr == nil {
					entry.Title = recipe.Title
				}
			}
		}
		files = append(files, entry)
		return nil
	})
	if walkErr != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("walk recipe repo: %v", walkErr))
	}
	return &GooseRecipeCandidatesResponse{Files: files, Truncated: truncated}, nil
}
