package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// LookupProject resolves either a canonical project ID or an exact project
// name. Without an organization reference it searches the caller's personal
// projects and every organization they belong to. Duplicate names are rejected
// so a command never silently targets the wrong project.
func LookupProject(ctx context.Context, apiClient client.Client, projectRef, orgRef string) (*types.Project, error) {
	projectRef = strings.TrimSpace(projectRef)
	if projectRef == "" {
		return nil, fmt.Errorf("project is required")
	}
	if strings.HasPrefix(projectRef, system.ProjectPrefix) {
		project, err := apiClient.GetProject(ctx, projectRef)
		if err != nil {
			return nil, fmt.Errorf("failed to get project %s: %w", projectRef, err)
		}
		return project, nil
	}

	projects := make([]*types.Project, 0)
	if orgRef != "" {
		org, err := LookupOrganization(ctx, apiClient, orgRef)
		if err != nil {
			return nil, err
		}
		orgProjects, err := apiClient.ListProjects(ctx, org.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list projects in %s: %w", org.Name, err)
		}
		projects = append(projects, orgProjects...)
	} else {
		personalProjects, err := apiClient.ListProjects(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("failed to list personal projects: %w", err)
		}
		projects = append(projects, personalProjects...)

		organizations, err := apiClient.ListOrganizations(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list organizations: %w", err)
		}
		for _, org := range organizations {
			orgProjects, err := apiClient.ListProjects(ctx, org.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to list projects in %s: %w", org.Name, err)
			}
			projects = append(projects, orgProjects...)
		}
	}

	matchesByID := make(map[string]*types.Project)
	for _, project := range projects {
		if project != nil && (project.Name == projectRef || project.ID == projectRef) {
			matchesByID[project.ID] = project
		}
	}
	if len(matchesByID) == 0 {
		return nil, fmt.Errorf("project not found: %s", projectRef)
	}
	if len(matchesByID) == 1 {
		for _, project := range matchesByID {
			return project, nil
		}
	}

	ids := make([]string, 0, len(matchesByID))
	for id := range matchesByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return nil, fmt.Errorf("project name %q is ambiguous; use one of these IDs: %s", projectRef, strings.Join(ids, ", "))
}
