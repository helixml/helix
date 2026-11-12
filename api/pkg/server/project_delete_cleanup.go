package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// releaseProjectOwnedResources removes or detaches the rows a project owns so
// none of them stays reachable once the project is deleted:
//
//   - repositories minted for the project (their id is "<project id>-…", the
//     convention of project_internal_repo_service and the sample-project
//     forks) are deleted, the same way organization deletion deletes its
//     repositories, unless another project also uses them;
//   - every project↔repository link is removed, so repositories the user
//     created on their own and attached stay in the organization, unlinked;
//   - access grants on the project are deleted;
//   - scheduled triggers that create work in the project are disabled (kept,
//     so their configuration and history are not lost).
//
// Chat sessions recorded against the project keep their project_id: they are
// the user's conversation history and stay readable by their owner.
func (s *HelixAPIServer) releaseProjectOwnedResources(ctx context.Context, project *types.Project) error {
	if err := s.releaseProjectRepositories(ctx, project); err != nil {
		return err
	}

	if project.OrganizationID != "" {
		grants, err := s.Store.ListAccessGrants(ctx, &store.ListAccessGrantsQuery{
			OrganizationID: project.OrganizationID,
			ResourceID:     project.ID,
		})
		if err != nil {
			return fmt.Errorf("list project access grants: %w", err)
		}
		for _, grant := range grants {
			if err := s.Store.DeleteAccessGrant(ctx, grant.ID); err != nil {
				return fmt.Errorf("delete project access grant %s: %w", grant.ID, err)
			}
		}
	}

	return s.disableProjectTriggers(ctx, project)
}

func (s *HelixAPIServer) releaseProjectRepositories(ctx context.Context, project *types.Project) error {
	links, err := s.Store.ListProjectRepositories(ctx, &types.ListProjectRepositoriesQuery{ProjectID: project.ID})
	if err != nil {
		return fmt.Errorf("list project repositories: %w", err)
	}

	derivedPrefix := project.ID + "-"
	for _, link := range links {
		if !strings.HasPrefix(link.RepositoryID, derivedPrefix) {
			continue
		}
		users, err := s.Store.ListProjectRepositories(ctx, &types.ListProjectRepositoriesQuery{RepositoryID: link.RepositoryID})
		if err != nil {
			return fmt.Errorf("list projects using repository %s: %w", link.RepositoryID, err)
		}
		sharedWithAnotherProject := false
		for _, u := range users {
			if u.ProjectID != project.ID {
				sharedWithAnotherProject = true
				break
			}
		}
		if sharedWithAnotherProject {
			log.Info().Str("project_id", project.ID).Str("repo_id", link.RepositoryID).
				Msg("keeping project repository that another project still uses")
			continue
		}
		if err := s.gitRepositoryService.DeleteRepository(ctx, link.RepositoryID); err != nil {
			return fmt.Errorf("delete project repository %s: %w", link.RepositoryID, err)
		}
		log.Info().Str("project_id", project.ID).Str("repo_id", link.RepositoryID).
			Msg("deleted repository created for the deleted project")
	}

	if err := s.Store.DeleteProjectRepositoriesByProject(ctx, project.ID); err != nil {
		return fmt.Errorf("detach project repositories: %w", err)
	}
	return nil
}

// disableProjectTriggers turns off enabled scheduled triggers whose action
// creates spec tasks in the project. Run against a deleted project they could
// only fail on every tick.
func (s *HelixAPIServer) disableProjectTriggers(ctx context.Context, project *types.Project) error {
	query := &store.ListTriggerConfigurationsQuery{TriggerType: types.TriggerTypeCron, Enabled: true}
	if project.OrganizationID != "" {
		query.OrganizationID = project.OrganizationID
	} else {
		query.Owner = project.UserID
	}
	triggers, err := s.Store.ListTriggerConfigurations(ctx, query)
	if err != nil {
		return fmt.Errorf("list project triggers: %w", err)
	}
	for _, trigger := range triggers {
		if trigger.Trigger.Cron == nil || trigger.Trigger.Cron.ProjectID != project.ID || !trigger.Enabled {
			continue
		}
		trigger.Enabled = false
		if _, err := s.Store.UpdateTriggerConfiguration(ctx, trigger); err != nil {
			return fmt.Errorf("disable trigger %s: %w", trigger.ID, err)
		}
		log.Info().Str("project_id", project.ID).Str("trigger_id", trigger.ID).
			Msg("disabled trigger that targeted the deleted project")
	}
	return nil
}
