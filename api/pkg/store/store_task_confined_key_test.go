package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) createConfinedTask(project *types.Project, confined bool) *types.SpecTask {
	task := &types.SpecTask{
		ID: "task-" + system.GenerateUUID(), ProjectID: project.ID, Name: "lane",
		Type: "feature", Priority: types.SpecTaskPriorityMedium, Status: types.TaskStatusBacklog,
		CreatedBy: "test-user", CreatedAt: time.Now(), UpdatedAt: time.Now(), ConfineAgentKey: confined,
	}
	suite.Require().NoError(suite.db.CreateSpecTask(suite.ctx, task))
	return task
}

// confine_agent_key is fixed at creation: a full-row update with the other
// value leaves the stored flag alone, whichever way it points.
func (suite *PostgresStoreTestSuite) TestUpdateSpecTask_KeepsConfineAgentKey() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() { _ = suite.db.DeleteProject(context.Background(), project.ID) })

	for _, confined := range []bool{true, false} {
		task := suite.createConfinedTask(project, confined)
		task.ConfineAgentKey = !confined
		task.Name = "renamed"
		suite.Require().NoError(suite.db.UpdateSpecTask(suite.ctx, task))

		stored, err := suite.db.GetSpecTask(suite.ctx, task.ID)
		suite.Require().NoError(err)
		suite.Equal(confined, stored.ConfineAgentKey)
		suite.Equal("renamed", stored.Name, "the rest of the update still applies")
	}
}

func (suite *PostgresStoreTestSuite) TestListSpecTasks_FiltersByID() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() { _ = suite.db.DeleteProject(context.Background(), project.ID) })
	finder := suite.createConfinedTask(project, true)
	suite.createConfinedTask(project, false)

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{ProjectID: project.ID, ID: finder.ID})
	suite.Require().NoError(err)
	suite.Require().Len(tasks, 1)
	suite.Equal(finder.ID, tasks[0].ID)
}

func (suite *PostgresStoreTestSuite) TestListSessions_FiltersBySpecTask() {
	owner := "usr-" + system.GenerateUUID()
	projectID := "proj-" + system.GenerateUUID()
	for _, taskID := range []string{"spt_finder", "spt_verifier", ""} {
		session := types.Session{
			ID: system.GenerateSessionID(), Owner: owner, OwnerType: types.OwnerTypeUser,
			ProjectID: projectID, Created: time.Now(), Updated: time.Now(),
			Metadata: types.SessionMetadata{SpecTaskID: taskID},
		}
		_, err := suite.db.CreateSession(suite.ctx, session)
		suite.Require().NoError(err)
		suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(context.Background(), session.ID) })
	}

	sessions, _, err := suite.db.ListSessions(suite.ctx, ListSessionsQuery{
		Owner: owner, OwnerType: types.OwnerTypeUser, RestrictToProjects: true,
		ProjectIDs: []string{projectID}, SpecTaskID: "spt_finder",
	})
	suite.Require().NoError(err)
	suite.Require().Len(sessions, 1)
	suite.Equal("spt_finder", sessions[0].Metadata.SpecTaskID)
}

func (suite *PostgresStoreTestSuite) TestAPIKey_TaskConfinedRoundTrips() {
	key := &types.ApiKey{
		Key: types.APIKeyPrefix + system.GenerateUUID(), Owner: "usr-" + system.GenerateUUID(),
		OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeAPI, Name: "Session key",
		ProjectID: "prj_a", SpecTaskID: "spt_finder", SessionID: "ses_finder", TaskConfined: true,
	}
	_, err := suite.db.CreateAPIKey(suite.ctx, key)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _ = suite.db.DeleteAPIKey(context.Background(), key.Key) })

	stored, err := suite.db.GetAPIKey(suite.ctx, &types.ApiKey{Key: key.Key})
	suite.Require().NoError(err)
	suite.True(stored.TaskConfined)
}

func (suite *PostgresStoreTestSuite) TestListArtifacts_FiltersBySpecTask() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() { _ = suite.db.DeleteProject(context.Background(), project.ID) })
	for _, taskID := range []string{"spt_finder", "spt_verifier", ""} {
		artifactID, versionID := "art_"+system.GenerateUUID(), "artv_"+system.GenerateUUID()
		artifact := &types.Artifact{
			ID: artifactID, ProjectID: project.ID, Name: "a", Kind: "html", Entrypoint: "index.html",
			Visibility: types.ArtifactVisibilityProject, ActiveVersionID: versionID,
			CreatedBy: "u", UpdatedBy: "u", SpecTaskID: taskID,
		}
		version := &types.ArtifactVersion{
			ID: versionID, ArtifactID: artifactID, Version: 1, StoragePrefix: "p",
			Files: []types.ArtifactFile{{Path: "index.html"}}, FileCount: 1, CreatedBy: "u",
		}
		suite.Require().NoError(suite.db.CreateArtifact(suite.ctx, artifact, version))
	}

	artifacts, err := suite.db.ListArtifacts(suite.ctx, &ListArtifactsQuery{ProjectID: project.ID, SpecTaskID: "spt_verifier"})
	suite.Require().NoError(err)
	suite.Require().Len(artifacts, 1)
	suite.Equal("spt_verifier", artifacts[0].SpecTaskID)

	all, err := suite.db.ListArtifacts(suite.ctx, &ListArtifactsQuery{ProjectID: project.ID})
	suite.Require().NoError(err)
	suite.Len(all, 3)
}
