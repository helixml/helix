package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// deleteUserAccount removes a user and everything that belongs to the user
// alone. Order matters:
//
//  1. Refuse, before changing anything, when the user is the only owner of an
//     organization that has other members: deleting them would leave that
//     organization without an owner. An admin transfers ownership first.
//  2. Revoke every API key, so the account stops authenticating immediately
//     even if a later teardown step fails.
//  3. Tear down personal runtime state the database transaction cannot reach:
//     personal chat sessions and desktops, personal projects (tasks, web
//     services, artifacts), personal repositories, organizations the user is
//     the only member of, and the user's and their personal agents' files.
//  4. Delete the remaining rows in one transaction (store.DeleteUser).
//
// Resources owned by an organization the user still shares with others are
// kept; removing the user's memberships and grants revokes their access.
func (apiServer *HelixAPIServer) deleteUserAccount(ctx context.Context, user *types.User) *system.HTTPError {
	soleMemberOrgs, httpErr := apiServer.userOrganizationsToDelete(ctx, user.ID)
	if httpErr != nil {
		return httpErr
	}

	if err := apiServer.Store.RevokeUserAPIKeys(ctx, user.ID); err != nil {
		return system.NewHTTPError500("failed to revoke API keys: " + err.Error())
	}

	if err := apiServer.deletePersonalSessions(ctx, user.ID); err != nil {
		return system.NewHTTPError500(err.Error())
	}

	projects, err := apiServer.Store.ListProjects(ctx, &store.ListProjectsQuery{UserID: user.ID})
	if err != nil {
		return system.NewHTTPError500("failed to list personal projects: " + err.Error())
	}
	for _, project := range projects {
		if httpErr := apiServer.purgeProject(ctx, project.ID); httpErr != nil {
			return system.NewHTTPError500(fmt.Sprintf("failed to delete personal project %s: %s", project.ID, httpErr.Message))
		}
	}

	repositories, err := apiServer.Store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{OwnerID: user.ID})
	if err != nil {
		return system.NewHTTPError500("failed to list repositories: " + err.Error())
	}
	for _, repository := range repositories {
		if repository.OrganizationID != "" {
			continue // organization-owned: kept
		}
		if err := apiServer.gitRepositoryService.DeleteRepository(ctx, repository.ID); err != nil {
			return system.NewHTTPError500(fmt.Sprintf("failed to delete personal repository %s: %s", repository.ID, err))
		}
	}

	for _, orgID := range soleMemberOrgs {
		if err := apiServer.purgeOrganization(ctx, orgID); err != nil {
			return system.NewHTTPError500(fmt.Sprintf("failed to delete organization %s: %s", orgID, err))
		}
	}

	if err := apiServer.deleteUserFiles(ctx, user.ID); err != nil {
		return system.NewHTTPError500(err.Error())
	}

	if err := apiServer.Store.DeleteUser(ctx, user.ID); err != nil {
		return system.NewHTTPError500("failed to delete user: " + err.Error())
	}
	return nil
}

// userOrganizationsToDelete returns the organizations the user is the only
// member of; they go with the user. It refuses when the user is the only
// owner of an organization other members still use.
func (apiServer *HelixAPIServer) userOrganizationsToDelete(ctx context.Context, userID string) ([]string, *system.HTTPError) {
	memberships, err := apiServer.Store.ListOrganizationMemberships(ctx, &store.ListOrganizationMembershipsQuery{UserID: userID})
	if err != nil {
		return nil, system.NewHTTPError500("failed to list organization memberships: " + err.Error())
	}
	var soleMember []string
	for _, membership := range memberships {
		members, err := apiServer.Store.ListOrganizationMemberships(ctx, &store.ListOrganizationMembershipsQuery{
			OrganizationID: membership.OrganizationID,
		})
		if err != nil {
			return nil, system.NewHTTPError500("failed to list organization members: " + err.Error())
		}
		otherMembers, otherOwners := 0, 0
		for _, member := range members {
			if member.UserID == userID {
				continue
			}
			otherMembers++
			if member.Role == types.OrganizationRoleOwner {
				otherOwners++
			}
		}
		switch {
		case otherMembers == 0:
			soleMember = append(soleMember, membership.OrganizationID)
		case membership.Role == types.OrganizationRoleOwner && otherOwners == 0:
			return nil, system.NewHTTPError409(fmt.Sprintf(
				"user is the only owner of organization %s, which has other members; make another member an owner first",
				membership.OrganizationID))
		}
	}
	return soleMember, nil
}

// deletePersonalSessions stops the desktops of the user's personal (non
// organization) sessions and soft-deletes the sessions.
func (apiServer *HelixAPIServer) deletePersonalSessions(ctx context.Context, userID string) error {
	sessions, _, err := apiServer.Store.ListSessions(ctx, store.ListSessionsQuery{
		Owner:                 userID,
		OwnerType:             types.OwnerTypeUser,
		IncludeExternalAgents: true,
	})
	if err != nil {
		return fmt.Errorf("list personal sessions: %w", err)
	}
	for _, session := range sessions {
		if session.Metadata.DevContainerID != "" {
			err := apiServer.externalAgentExecutor.StopDesktop(ctx, session.ID)
			if err != nil && !sandboxHostGone(err, session.ID) {
				return fmt.Errorf("stop desktop %s: %w", session.ID, err)
			}
		}
		if _, err := apiServer.Store.DeleteSession(ctx, session.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("delete session %s: %w", session.ID, err)
		}
	}
	return nil
}

// deleteUserFiles removes the user's filestore directory and those of their
// personal agents (deleted with the user's rows).
func (apiServer *HelixAPIServer) deleteUserFiles(ctx context.Context, userID string) error {
	apps, err := apiServer.Store.ListApps(ctx, &store.ListAppsQuery{Owner: userID, OwnerType: types.OwnerTypeUser})
	if err != nil {
		return fmt.Errorf("list personal agents: %w", err)
	}
	prefix := apiServer.Controller.Options.Config.Controller.FilePrefixGlobal
	paths := []string{filestore.GetUserPrefix(prefix, userID)}
	for _, app := range apps {
		if app.OrganizationID == "" {
			paths = append(paths, filestore.GetAppPrefix(prefix, app.ID))
		}
	}
	for _, path := range paths {
		// The trailing slash makes object stores delete the whole prefix.
		if err := apiServer.Controller.Options.Filestore.Delete(ctx, path+"/"); err != nil {
			return fmt.Errorf("delete files %s: %w", path, err)
		}
		log.Info().Str("user_id", userID).Str("path", path).Msg("deleted user files")
	}
	return nil
}
