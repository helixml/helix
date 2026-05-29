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
	"github.com/helixml/helix/api/pkg/store"
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

	rootDir, repoErr := s.resolveGooseRecipeRoot(ctx, app, assistant, project)
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

// GooseRecipeCandidatesResponse wraps the file list so the response is
// extensible (truncation flag, error message) without an API break.
// The Repositories list lets the frontend render a "recipe repository"
// dropdown without making a second roundtrip to /projects/:id/repositories.
type GooseRecipeCandidatesResponse struct {
	Files     []GooseRecipeCandidate `json:"files"`
	Truncated bool                   `json:"truncated,omitempty"`
	Error     string                 `json:"error,omitempty"`

	// Repositories attached to the project, eligible to host recipes.
	Repositories []GooseRecipeRepoOption `json:"repositories"`
	// CurrentRepoURL is the external_url of the repo whose files we
	// walked. Empty when walking the project's primary repo.
	CurrentRepoURL string `json:"current_repo_url,omitempty"`
	// ProjectID lets the editor deep-link to the parent project's
	// Repositories tab without an extra roundtrip.
	ProjectID string `json:"project_id,omitempty"`
	// OrgID is the parent project's org — needed for the
	// /orgs/:org_id/... deep-link URL.
	OrgID string `json:"org_id,omitempty"`
}

// GooseRecipeRepoOption is one entry in the recipe-repo dropdown.
// URL is the value persisted onto GooseRecipeRepoURL; empty URL is the
// "(Use primary repository)" entry the UI renders implicitly.
type GooseRecipeRepoOption struct {
	URL       string `json:"url"`
	Name      string `json:"name"`
	IsPrimary bool   `json:"is_primary,omitempty"`
}

// listProjectGooseRecipeCandidates godoc
// @Summary List YAML files in the project's recipe repository
// @Description Walks the recipe repo's local clone and returns every
// @Description *.yaml / *.yml file the user could pick as a Goose recipe.
// @Description Skips noisy directories (node_modules, vendor, .git) and
// @Description caps at 200 entries. Used by the project-settings file
// @Description picker so users don't have to type recipe paths by hand.
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} GooseRecipeCandidatesResponse
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/goose-recipes/candidates [get]
func (s *HelixAPIServer) listProjectGooseRecipeCandidates(_ http.ResponseWriter, r *http.Request) (*GooseRecipeCandidatesResponse, *system.HTTPError) {
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
		return &GooseRecipeCandidatesResponse{Files: []GooseRecipeCandidate{}}, nil
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
	if assistant == nil {
		return &GooseRecipeCandidatesResponse{Files: []GooseRecipeCandidate{}}, nil
	}

	return s.buildRecipeCandidates(ctx, app, assistant, project)
}

// resolveGooseRecipeRoot returns the absolute container-side directory
// containing the agent's recipe files. Returns ("", "human reason") when
// the repo can't be located so the API can render a friendly UI message.
//
// Mirrors the lookup order used by resolveGooseRecipesIntoConfig at
// session-start time: assistant.GooseRecipeRepoURL wins; otherwise fall
// back to the project's primary repo. Returning a string error instead
// of an `error` keeps the HTTP layer thin — these messages are surfaced
// verbatim to the user.
func (s *HelixAPIServer) resolveGooseRecipeRoot(ctx context.Context, app *types.App, assistant *types.AssistantConfig, project *types.Project) (string, string) {
	var repo *types.GitRepository
	if assistant.GooseRecipeRepoURL != "" {
		r, err := s.Store.GetGitRepositoryByExternalURL(ctx, app.OrganizationID, assistant.GooseRecipeRepoURL)
		if err != nil {
			return "", fmt.Sprintf("recipe repo %s not attached to this org", assistant.GooseRecipeRepoURL)
		}
		repo = r
	} else {
		if project.DefaultRepoID == "" {
			return "", "project has no primary repository for recipes to live in"
		}
		r, err := s.Store.GetGitRepository(ctx, project.DefaultRepoID)
		if err != nil {
			return "", "project's primary repository could not be loaded"
		}
		repo = r
	}
	if repo.LocalPath == "" {
		return "", "recipe repository has not been cloned yet"
	}
	return repo.LocalPath, ""
}

// listAppGooseRecipeCandidates godoc
// @Summary List YAML files in an app's recipe repository (by app id)
// @Description Same as the project-keyed variant, but reachable from the
// @Description app-edit UI where the parent project id isn't always in
// @Description scope. Resolves the parent project by finding one whose
// @Description default_helix_app_id matches this app — apps in Helix are
// @Description created 1:1 with a project's default agent, so this is
// @Description unambiguous in practice.
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
		return &GooseRecipeCandidatesResponse{Files: []GooseRecipeCandidate{}}, nil
	}

	// Find the project that uses this app as its default agent so we can
	// fall back to its primary repo when GooseRecipeRepoURL is empty. We
	// don't have a dedicated DB index for this lookup; org-scoped
	// ListProjects + in-memory filter is fine — agent apps are 1:1 with
	// projects in practice and the list stays small.
	project, projErr := s.findProjectByDefaultAppID(ctx, app.OrganizationID, appID)
	if projErr != nil {
		return &GooseRecipeCandidatesResponse{Files: []GooseRecipeCandidate{}, Error: projErr.Error()}, nil
	}

	return s.buildRecipeCandidates(ctx, app, assistant, project)
}

// buildRecipeCandidates resolves the recipe root, walks for *.yaml
// files, and decorates the response with the list of repos attached to
// the parent project so the UI can render a "recipe repository"
// dropdown. Walk errors are surfaced via Error, not HTTPError, because
// the dropdown is still useful even when the current repo has no files
// (or hasn't been cloned).
func (s *HelixAPIServer) buildRecipeCandidates(ctx context.Context, app *types.App, assistant *types.AssistantConfig, project *types.Project) (*GooseRecipeCandidatesResponse, *system.HTTPError) {
	repos, err := s.Store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{ProjectID: project.ID})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("list project repos: %v", err))
	}
	options := make([]GooseRecipeRepoOption, 0, len(repos))
	for _, repo := range repos {
		opt := GooseRecipeRepoOption{
			Name:      repo.Name,
			IsPrimary: repo.ID == project.DefaultRepoID,
		}
		// External repos have a stable ExternalURL we can persist on
		// GooseRecipeRepoURL. Helix-hosted repos use CloneURL as the
		// identifier — but those are uncommon for recipe storage so we
		// don't go out of our way to surface them; the user can still
		// hit them by leaving the field empty (primary fallback).
		if repo.IsExternal && repo.ExternalURL != "" {
			opt.URL = repo.ExternalURL
		} else {
			opt.URL = repo.CloneURL
		}
		options = append(options, opt)
	}

	// Surface the URL whose tree we're about to walk so the frontend can
	// pre-select the right dropdown entry.
	currentURL := assistant.GooseRecipeRepoURL

	rootDir, repoErr := s.resolveGooseRecipeRoot(ctx, app, assistant, project)
	if repoErr != "" {
		return &GooseRecipeCandidatesResponse{
			Files:          []GooseRecipeCandidate{},
			Error:          repoErr,
			Repositories:   options,
			CurrentRepoURL: currentURL,
			ProjectID:      project.ID,
			OrgID:          project.OrganizationID,
		}, nil
	}
	walked, walkErr := walkRecipeCandidates(rootDir)
	if walkErr != nil {
		return nil, walkErr
	}
	walked.Repositories = options
	walked.CurrentRepoURL = currentURL
	walked.ProjectID = project.ID
	walked.OrgID = project.OrganizationID
	return walked, nil
}

// findProjectByDefaultAppID returns the project that uses appID as its
// default helix app within the given org. Errors with a human-friendly
// reason when no such project exists.
func (s *HelixAPIServer) findProjectByDefaultAppID(ctx context.Context, orgID, appID string) (*types.Project, error) {
	projects, err := s.Store.ListProjects(ctx, &store.ListProjectsQuery{OrganizationID: orgID})
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	for _, p := range projects {
		if p.DefaultHelixAppID == appID {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no project uses this app as its default agent")
}

// walkRecipeCandidates is the on-disk walk shared by the project- and
// app-keyed candidate handlers. Extracted to keep the two handlers'
// auth/lookup logic distinct without duplicating the walker.
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
