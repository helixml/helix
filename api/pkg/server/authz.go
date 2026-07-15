package server

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/orgstore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// orgAuthorizer builds an org authorizer over the server's store. The org
// authorization logic lives in api/pkg/orgstore (shared with downstream
// services); these methods delegate to it so there is a single implementation.
func (apiServer *HelixAPIServer) orgAuthorizer() *orgstore.Authorizer {
	return orgstore.NewAuthorizer(apiServer.Store)
}

// authorizeOrgOwner checks if the user is an owner of the organization.
func (apiServer *HelixAPIServer) authorizeOrgOwner(ctx context.Context, user *types.User, orgID string) (*types.OrganizationMembership, error) {
	return apiServer.orgAuthorizer().AuthorizeOrgOwner(ctx, user, orgID)
}

// authorizeOrgMember checks if the user is a member of the organization.
func (apiServer *HelixAPIServer) authorizeOrgMember(ctx context.Context, user *types.User, orgID string) (*types.OrganizationMembership, error) {
	return apiServer.orgAuthorizer().AuthorizeOrgMember(ctx, user, orgID)
}

func (apiServer *HelixAPIServer) resolveOrgID(ctx context.Context, orgRef string) (string, error) {
	if orgRef == "" {
		return "", fmt.Errorf("organization ID is required")
	}

	org, err := apiServer.lookupOrg(ctx, orgRef)
	if err != nil {
		return "", err
	}

	return org.ID, nil
}

// authorizeUserToAppAccessGrants checks if the user is a member of the organization or the app owner
// and has the necessary permissions to perform the action on the access grant
func (apiServer *HelixAPIServer) authorizeUserToAppAccessGrants(ctx context.Context, user *types.User, app *types.App, action types.Action) error {
	// Check if user is a member of the org
	orgMembership, err := apiServer.authorizeOrgMember(ctx, user, app.OrganizationID)
	if err != nil {
		return err
	}

	// App owner can always access the app
	if user.ID == app.Owner {
		return nil
	}

	// Org owner can always access the app
	if orgMembership.Role == types.OrganizationRoleOwner {
		return nil
	}

	return apiServer.authorizeUserToResource(ctx, user, app.OrganizationID, app.ID, types.ResourceAccessGrants, action)
}

// authorizeUserToApp checks if a user has access to an app
// This is a server-level method that centralizes the authorization logic
func (apiServer *HelixAPIServer) authorizeUserToApp(ctx context.Context, user *types.User, app *types.App, action types.Action) error {
	// If the organization ID is not set and the user is not the app owner, then error
	if app.OrganizationID == "" {
		// This is the old style app logic, where the app is owned by a user and optionally made global

		// If the user is the owner of the app, they can access it
		if user.ID == app.Owner {
			return nil
		}

		// If the app is global, the user can access it
		if app.Global {
			// But only admins can update or delete global apps
			if action == types.ActionUpdate || action == types.ActionDelete {
				if !isAdmin(user) {
					return fmt.Errorf("only admin users can update or delete global apps")
				}
			}

			// If the app is global, the user can access it
			return nil
		}

		// Otherwise the user is not allowed to access the app
		return fmt.Errorf("user is not the owner of the app")
	}

	// If organization ID is set, authorize the user against the organization
	orgMembership, err := apiServer.authorizeOrgMember(ctx, user, app.OrganizationID)
	if err != nil {
		return err
	}

	// App owner can always access the app
	if user.ID == app.Owner {
		return nil
	}

	// Org owner can always access the app
	if orgMembership.Role == types.OrganizationRoleOwner {
		return nil
	}

	err = apiServer.authorizeUserToResource(ctx, user, app.OrganizationID, app.ID, types.ResourceApplication, action)
	if err == nil {
		return nil
	}

	// List all projects in the org and check if the app is referenced by any project
	// the user has access to. We can't filter by UserID here because that only matches
	// the project owner, not users who were granted access via RBAC.
	projects, err := apiServer.Store.ListProjects(ctx, &store.ListProjectsQuery{
		OrganizationID: app.OrganizationID,
	})
	if err != nil {
		return err
	}

	for _, project := range projects {
		if project.DefaultHelixAppID != app.ID &&
			project.ProjectManagerHelixAppID != app.ID &&
			project.PullRequestReviewerHelixAppID != app.ID {
			continue
		}
		// App is referenced by this project — check if user has access to the project
		if err := apiServer.authorizeUserToProject(ctx, user, project, types.ActionGet); err == nil {
			return nil
		}
	}

	return fmt.Errorf("user does not have access to the app")
}

// authorizeUserToProjectByID helper function to authorize a user to a project by ID, used
// for RBAC for project sub-resources such as spec-driven tasks, work sessions, etc
func (apiServer *HelixAPIServer) authorizeUserToProjectByID(ctx context.Context, user *types.User, projectID string, action types.Action) error {
	if projectID == "" {
		return fmt.Errorf("project ID is required")
	}

	project, err := apiServer.Store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}

	return apiServer.authorizeUserToProject(ctx, user, project, action)
}

func (apiServer *HelixAPIServer) authorizeUserToProject(ctx context.Context, user *types.User, project *types.Project, action types.Action) error {
	// If the organization ID is not set and the user is not the project owner, then error
	if project.OrganizationID == "" {
		// This is the old style project logic, where the project is owned by a user and optionally made global

		// If the user is the owner of the project, they can access it
		if user.ID == project.UserID {
			return nil
		}

		// Otherwise the user is not allowed to access the app
		return fmt.Errorf("user is not the owner of the app")
	}

	// If organization ID is set, authorize the user against the organization
	orgMembership, err := apiServer.authorizeOrgMember(ctx, user, project.OrganizationID)
	if err != nil {
		return err
	}

	// Project owner can always access the project (they still have to have
	// org membership)
	if user.ID == project.UserID {
		return nil
	}

	// Org owner can always access the app
	if orgMembership.Role == types.OrganizationRoleOwner {
		return nil
	}
	if project.Metadata.OrgMembersAccess && (action == types.ActionGet || action == types.ActionList || action == types.ActionUseAction) {
		return nil
	}

	return apiServer.authorizeUserToResource(ctx, user, project.OrganizationID, project.ID, types.ResourceProject, action)
}

func (apiServer *HelixAPIServer) authorizeUserToRepository(ctx context.Context, user *types.User, repository *types.GitRepository, action types.Action) error {
	// If the organization ID is not set, only the owner can access
	if repository.OrganizationID == "" {
		if user.ID == repository.OwnerID {
			return nil
		}
		return fmt.Errorf("user is not the owner of the repository")
	}

	// If organization ID is set, authorize the user against the organization
	orgMembership, err := apiServer.authorizeOrgMember(ctx, user, repository.OrganizationID)
	if err != nil {
		return err
	}

	// Repository owner can always access the repository (they still have to have
	// org membership)
	if user.ID == repository.OwnerID {
		return nil
	}

	// Org owner can always access the repository
	if orgMembership.Role == types.OrganizationRoleOwner {
		return nil
	}

	err = apiServer.authorizeUserToResource(ctx, user, repository.OrganizationID, repository.ID, types.ResourceGitRepository, action)
	if err == nil {
		return nil
	}

	// List all projects in the org and check if the repository is attached to any project
	// the user has access to. We can't filter by UserID here because that only matches
	// the project owner, not users who were granted access via RBAC.
	projects, err := apiServer.Store.ListProjects(ctx, &store.ListProjectsQuery{
		OrganizationID: repository.OrganizationID,
	})
	if err != nil {
		return err
	}

	for _, project := range projects {
		projectRepositories, err := apiServer.Store.ListProjectRepositories(ctx, &types.ListProjectRepositoriesQuery{
			ProjectID:    project.ID,
			RepositoryID: repository.ID,
		})
		if err != nil {
			return err
		}
		for _, projectRepository := range projectRepositories {
			if projectRepository.RepositoryID == repository.ID {
				// Repo is attached to this project — check if user has access to the project
				if err := apiServer.authorizeUserToProject(ctx, user, project, action); err == nil {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("user does not have access to the repository")
}

func (apiServer *HelixAPIServer) authorizeUserToSession(ctx context.Context, user *types.User, session *types.Session, action types.Action) error {
	// If the organization ID is not set and the user is not the project owner, then error
	if session.OrganizationID == "" {
		// This is the old style project logic, where the project is owned by a user and optionally made global

		// If the user is the owner of the project, they can access it
		if user.ID == session.Owner {
			return nil
		}

		// Otherwise the user is not allowed to access the app
		return fmt.Errorf("user is not the owner of the app")
	}

	// If organization ID is set, authorize the user against the organization
	orgMembership, err := apiServer.authorizeOrgMember(ctx, user, session.OrganizationID)
	if err != nil {
		return err
	}

	// Project owner can always access the project (they still have to have
	// org membership)
	if user.ID == session.Owner {
		return nil
	}

	// Org owner can always access the app
	if orgMembership.Role == types.OrganizationRoleOwner {
		return nil
	}

	if session.ProjectID == "" {
		return fmt.Errorf("not authorized to access session without a project")
	}

	project, err := apiServer.Store.GetProject(ctx, session.ProjectID)
	if err != nil {
		return err
	}
	if project.Metadata.OrgMembersAccess && action == types.ActionUpdate {
		return apiServer.authorizeUserToProject(ctx, user, project, types.ActionGet)
	}

	return apiServer.authorizeUserToProject(ctx, user, project, action)
}

// authorizeUserToResource evaluates the user's team + direct access grants for a
// resource. Delegates to the shared orgstore authorizer.
func (apiServer *HelixAPIServer) authorizeUserToResource(ctx context.Context, user *types.User, orgID, resourceID string, resourceType types.Resource, action types.Action) error {
	return apiServer.orgAuthorizer().AuthorizeUserToResource(ctx, user, orgID, resourceID, resourceType, action)
}
