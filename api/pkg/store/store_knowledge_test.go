package store

import (
	"context"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestPostgresStore_CreateKnowledge() {
	knowledge := types.Knowledge{
		ID:    system.GenerateKnowledgeID(),
		Owner: "user_id",
		Name:  "Test Knowledge",
	}

	createdKnowledge, err := suite.db.CreateKnowledge(context.Background(), &knowledge)

	suite.NoError(err)
	suite.NotNil(createdKnowledge)
	suite.Equal(knowledge.ID, createdKnowledge.ID)
	suite.Equal(knowledge.Owner, createdKnowledge.Owner)
	suite.Equal(knowledge.Name, createdKnowledge.Name)
	suite.NotZero(createdKnowledge.Created)
	suite.NotZero(createdKnowledge.Updated)

	// Cleanup
	err = suite.db.DeleteKnowledge(context.Background(), knowledge.ID)
	suite.NoError(err)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetKnowledge() {
	knowledge := types.Knowledge{
		ID:    system.GenerateKnowledgeID(),
		Owner: "user_id",
		Name:  "Test Knowledge",
	}

	_, err := suite.db.CreateKnowledge(context.Background(), &knowledge)
	suite.NoError(err)

	retrievedKnowledge, err := suite.db.GetKnowledge(context.Background(), knowledge.ID)

	suite.NoError(err)
	suite.NotNil(retrievedKnowledge)
	suite.Equal(knowledge.ID, retrievedKnowledge.ID)
	suite.Equal(knowledge.Owner, retrievedKnowledge.Owner)
	suite.Equal(knowledge.Name, retrievedKnowledge.Name)

	// Cleanup
	err = suite.db.DeleteKnowledge(context.Background(), knowledge.ID)
	suite.NoError(err)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_LookupKnowledge() {
	knowledge := types.Knowledge{
		ID:    system.GenerateKnowledgeID(),
		Owner: "user_id",
		Name:  "Test Knowledge",
		AppID: "app_id",
	}

	_, err := suite.db.CreateKnowledge(context.Background(), &knowledge)
	suite.NoError(err)

	query := &LookupKnowledgeQuery{
		AppID: knowledge.AppID,
		ID:    knowledge.ID,
		Name:  knowledge.Name,
		Owner: knowledge.Owner,
	}

	retrievedKnowledge, err := suite.db.LookupKnowledge(context.Background(), query)

	suite.NoError(err)
	suite.NotNil(retrievedKnowledge)
	suite.Equal(knowledge.ID, retrievedKnowledge.ID)
	suite.Equal(knowledge.Owner, retrievedKnowledge.Owner)
	suite.Equal(knowledge.Name, retrievedKnowledge.Name)
	suite.Equal(knowledge.AppID, retrievedKnowledge.AppID)

	// Cleanup
	err = suite.db.DeleteKnowledge(context.Background(), knowledge.ID)
	suite.NoError(err)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateKnowledge() {
	knowledge := types.Knowledge{
		ID:    system.GenerateKnowledgeID(),
		Owner: "user_id",
		Name:  "Test Knowledge",
	}

	_, err := suite.db.CreateKnowledge(context.Background(), &knowledge)
	suite.NoError(err)

	knowledge.Name = "Updated Knowledge"
	updatedKnowledge, err := suite.db.UpdateKnowledge(context.Background(), &knowledge)

	suite.NoError(err)
	suite.NotNil(updatedKnowledge)
	suite.Equal(knowledge.ID, updatedKnowledge.ID)
	suite.Equal(knowledge.Owner, updatedKnowledge.Owner)
	suite.Equal("Updated Knowledge", updatedKnowledge.Name)

	// Cleanup
	err = suite.db.DeleteKnowledge(context.Background(), knowledge.ID)
	suite.NoError(err)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateKnowledgeState() {
	knowledge := types.Knowledge{
		ID:    system.GenerateKnowledgeID(),
		Owner: "user_id",
		Name:  "Test Knowledge",
	}

	_, err := suite.db.CreateKnowledge(context.Background(), &knowledge)
	suite.NoError(err)

	err = suite.db.UpdateKnowledgeState(context.Background(),
		knowledge.ID, types.KnowledgeStateIndexing, "Indexing")
	suite.NoError(err, "failed to update knowledge state")

	updatedKnowledge, err := suite.db.GetKnowledge(context.Background(), knowledge.ID)
	suite.NoError(err, "failed to get knowledge")
	suite.NotNil(updatedKnowledge)
	suite.Equal(knowledge.ID, updatedKnowledge.ID)
	suite.Equal(knowledge.Owner, updatedKnowledge.Owner)
	suite.Equal("Test Knowledge", updatedKnowledge.Name)

	// Cleanup
	err = suite.db.DeleteKnowledge(context.Background(), knowledge.ID)
	suite.NoError(err)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListKnowledge() {
	// Create multiple knowledge entries
	knowledge1 := types.Knowledge{ID: system.GenerateKnowledgeID(), Owner: "user_id", Name: "Knowledge 1", AppID: "app_id"}
	knowledge2 := types.Knowledge{ID: system.GenerateKnowledgeID(), Owner: "user_id", Name: "Knowledge 2", AppID: "app_id"}

	_, err := suite.db.CreateKnowledge(context.Background(), &knowledge1)
	suite.NoError(err)
	_, err = suite.db.CreateKnowledge(context.Background(), &knowledge2)
	suite.NoError(err)

	query := &ListKnowledgeQuery{
		Owner: "user_id",
		AppID: "app_id",
	}

	knowledgeList, err := suite.db.ListKnowledge(context.Background(), query)

	suite.NoError(err)
	suite.Len(knowledgeList, 2)

	// Verify that both created knowledge entries are in the list
	ids := []string{knowledgeList[0].ID, knowledgeList[1].ID}
	suite.Contains(ids, knowledge1.ID)
	suite.Contains(ids, knowledge2.ID)

	// Cleanup
	err = suite.db.DeleteKnowledge(context.Background(), knowledge1.ID)
	suite.NoError(err)

	err = suite.db.DeleteKnowledge(context.Background(), knowledge2.ID)
	suite.NoError(err)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_DeleteKnowledge() {
	knowledge := types.Knowledge{
		ID:    system.GenerateKnowledgeID(),
		Owner: "user_id",
		Name:  "Test Knowledge",
	}

	_, err := suite.db.CreateKnowledge(context.Background(), &knowledge)
	suite.NoError(err)

	// Add a knowledge version
	version := types.KnowledgeVersion{
		KnowledgeID: knowledge.ID,
		State:       types.KnowledgeStateIndexing,
	}

	_, err = suite.db.CreateKnowledgeVersion(context.Background(), &version)
	suite.NoError(err)

	err = suite.db.DeleteKnowledge(context.Background(), knowledge.ID)
	suite.NoError(err)

	// Verify that the knowledge is deleted
	_, err = suite.db.GetKnowledge(context.Background(), knowledge.ID)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	// Verify that the knowledge version is deleted
	_, err = suite.db.GetKnowledgeVersion(context.Background(), version.ID)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_CreateKnowledgeVersion() {
	knowledge := types.Knowledge{
		ID:    system.GenerateKnowledgeID(),
		Owner: "user_id",
		Name:  "Test Knowledge",
	}

	_, err := suite.db.CreateKnowledge(context.Background(), &knowledge)
	suite.NoError(err)

	version := types.KnowledgeVersion{
		KnowledgeID: knowledge.ID,
		State:       types.KnowledgeStateIndexing,
	}

	createdVersion, err := suite.db.CreateKnowledgeVersion(context.Background(), &version)

	suite.NoError(err)
	suite.NotNil(createdVersion)
	suite.Equal(version.KnowledgeID, createdVersion.KnowledgeID)
	suite.Equal(version.State, createdVersion.State)
	suite.NotZero(createdVersion.Created)
	suite.NotZero(createdVersion.Updated)

	// Cleanup
	err = suite.db.DeleteKnowledge(context.Background(), knowledge.ID)
	suite.NoError(err)
	err = suite.db.DeleteKnowledgeVersion(context.Background(), createdVersion.ID)
	suite.NoError(err)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetKnowledgeVersion() {
	knowledge := types.Knowledge{
		ID:    system.GenerateKnowledgeID(),
		Owner: "user_id",
		Name:  "Test Knowledge",
	}

	_, err := suite.db.CreateKnowledge(context.Background(), &knowledge)
	suite.NoError(err)

	version := types.KnowledgeVersion{
		KnowledgeID: knowledge.ID,
		State:       types.KnowledgeStateIndexing,
	}

	createdVersion, err := suite.db.CreateKnowledgeVersion(context.Background(), &version)
	suite.NoError(err)

	retrievedVersion, err := suite.db.GetKnowledgeVersion(context.Background(), createdVersion.ID)

	suite.NoError(err)
	suite.NotNil(retrievedVersion)
	suite.Equal(createdVersion.ID, retrievedVersion.ID)
	suite.Equal(createdVersion.KnowledgeID, retrievedVersion.KnowledgeID)
	suite.Equal(createdVersion.State, retrievedVersion.State)

	// Cleanup
	err = suite.db.DeleteKnowledge(context.Background(), knowledge.ID)
	suite.NoError(err)
	err = suite.db.DeleteKnowledgeVersion(context.Background(), createdVersion.ID)
	suite.NoError(err)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListKnowledgeVersions() {
	knowledge := types.Knowledge{
		ID:    system.GenerateKnowledgeID(),
		Owner: "user_id",
		Name:  "Test Knowledge",
	}

	_, err := suite.db.CreateKnowledge(context.Background(), &knowledge)
	suite.NoError(err)

	version1 := types.KnowledgeVersion{KnowledgeID: knowledge.ID, State: types.KnowledgeStateIndexing}
	version2 := types.KnowledgeVersion{KnowledgeID: knowledge.ID, State: types.KnowledgeStateReady}

	_, err = suite.db.CreateKnowledgeVersion(context.Background(), &version1)
	suite.NoError(err)
	_, err = suite.db.CreateKnowledgeVersion(context.Background(), &version2)
	suite.NoError(err)

	query := &ListKnowledgeVersionQuery{
		KnowledgeID: knowledge.ID,
	}

	versionList, err := suite.db.ListKnowledgeVersions(context.Background(), query)

	suite.NoError(err)
	suite.Len(versionList, 2)

	// Verify that both created versions are in the list
	states := []types.KnowledgeState{versionList[0].State, versionList[1].State}
	suite.Contains(states, types.KnowledgeStateIndexing)
	suite.Contains(states, types.KnowledgeStateReady)

	// Cleanup
	err = suite.db.DeleteKnowledge(context.Background(), knowledge.ID)
	suite.NoError(err)
	err = suite.db.DeleteKnowledgeVersion(context.Background(), versionList[0].ID)
	suite.NoError(err)
	err = suite.db.DeleteKnowledgeVersion(context.Background(), versionList[1].ID)
	suite.NoError(err)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_DeleteKnowledgeVersion() {
	knowledge := types.Knowledge{
		ID:    system.GenerateKnowledgeID(),
		Owner: "user_id",
		Name:  "Test Knowledge",
	}

	_, err := suite.db.CreateKnowledge(context.Background(), &knowledge)
	suite.NoError(err)

	version := types.KnowledgeVersion{
		KnowledgeID: knowledge.ID,
		State:       types.KnowledgeStateIndexing,
	}

	createdVersion, err := suite.db.CreateKnowledgeVersion(context.Background(), &version)
	suite.NoError(err)

	err = suite.db.DeleteKnowledgeVersion(context.Background(), createdVersion.ID)
	suite.NoError(err)

	// Verify that the version is deleted
	_, err = suite.db.GetKnowledgeVersion(context.Background(), createdVersion.ID)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	// Cleanup
	err = suite.db.DeleteKnowledge(context.Background(), knowledge.ID)
	suite.NoError(err)
}
