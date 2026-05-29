package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
