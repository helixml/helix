package store

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
)

func TestModelTestSuite(t *testing.T) {
	suite.Run(t, new(ModelTestSuite))
}

type ModelTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
}

func (suite *ModelTestSuite) SetupTest() {
	suite.ctx = context.Background()

	var storeCfg config.Store
	err := envconfig.Process("", &storeCfg)
	suite.NoError(err)

	store, err := NewPostgresStore(storeCfg)
	suite.Require().NoError(err)
	suite.db = store

	// Clean up database before each test
	suite.Require().NoError(suite.db.gdb.Exec("DELETE FROM models").Error)

	suite.T().Cleanup(func() {
		_ = suite.db.Close()
	})
}

func (suite *ModelTestSuite) TestCreateModel() {
	modelID := "test-model-" + system.GenerateAppID()
	validModel := &types.Model{
		ID:            modelID,
		Name:          "Test Model",
		Type:          types.ModelTypeChat,
		Runtime:       types.RuntimeOllama,
		ContextLength: 4096,
		Memory:        8 * 1024 * 1024 * 1024, // 8GB
		Description:   "A test model",
		Enabled:       true,
	}

	// Test creating a valid model
	createdModel, err := suite.db.CreateModel(suite.ctx, validModel)
	suite.NoError(err)
	suite.NotNil(createdModel)
	suite.Equal(validModel.ID, createdModel.ID)
	suite.Equal(validModel.Name, createdModel.Name)
	suite.Equal(validModel.Type, createdModel.Type)
	suite.Equal(validModel.Runtime, createdModel.Runtime)
	suite.Equal(validModel.ContextLength, createdModel.ContextLength)
	suite.Equal(validModel.Memory, createdModel.Memory)
	suite.Equal(validModel.Description, createdModel.Description)
	suite.Equal(validModel.Enabled, createdModel.Enabled)
	suite.NotZero(createdModel.Created)
	suite.NotZero(createdModel.Updated)

	// Test creating a model with missing ID
	invalidModelNoID := &types.Model{
		Name:          "Invalid Model",
		Type:          types.ModelTypeChat,
		Runtime:       types.RuntimeOllama,
		ContextLength: 4096,
		Memory:        8 * 1024 * 1024 * 1024,
	}
	_, err = suite.db.CreateModel(suite.ctx, invalidModelNoID)
	suite.Error(err)
	suite.Contains(err.Error(), "id not specified")

	// Test creating a model with missing Type
	invalidModelNoType := &types.Model{
		ID:            "invalid-model-no-type",
		Name:          "Invalid Model",
		Runtime:       types.RuntimeOllama,
		ContextLength: 4096,
		Memory:        8 * 1024 * 1024 * 1024,
	}
	_, err = suite.db.CreateModel(suite.ctx, invalidModelNoType)
	suite.Error(err)
	suite.Contains(err.Error(), "type not specified")

	// Test creating a model with missing Memory
	invalidModelNoMemory := &types.Model{
		ID:            "invalid-model-no-memory",
		Name:          "Invalid Model",
		Type:          types.ModelTypeChat,
		Runtime:       types.RuntimeOllama,
		ContextLength: 4096,
	}
	_, err = suite.db.CreateModel(suite.ctx, invalidModelNoMemory)
	suite.Error(err)
	suite.Contains(err.Error(), "memory not specified")

	// Test creating a chat model with missing ContextLength
	invalidModelNoContext := &types.Model{
		ID:      "invalid-model-no-context",
		Name:    "Invalid Model",
		Type:    types.ModelTypeChat,
		Runtime: types.RuntimeOllama,
		Memory:  8 * 1024 * 1024 * 1024,
	}
	_, err = suite.db.CreateModel(suite.ctx, invalidModelNoContext)
	suite.Error(err)
	suite.Contains(err.Error(), "context length not specified")
}

func (suite *ModelTestSuite) TestGetModel() {
	modelID := "test-get-model-" + system.GenerateAppID()
	model := &types.Model{
		ID:            modelID,
		Name:          "Get Test Model",
		Type:          types.ModelTypeImage,
		Runtime:       types.RuntimeDiffusers,
		Memory:        12 * 1024 * 1024 * 1024, // 12GB
		Description:   "Model for Get test",
		Enabled:       true,
		ContextLength: 0, // Image models have 0 context length
	}

	// Create model first
	_, err := suite.db.CreateModel(suite.ctx, model)
	suite.Require().NoError(err)

	// Test getting the existing model
	retrievedModel, err := suite.db.GetModel(suite.ctx, modelID)
	suite.NoError(err)
	suite.NotNil(retrievedModel)
	suite.Equal(model.ID, retrievedModel.ID)
	suite.Equal(model.Name, retrievedModel.Name)

	// Test getting a non-existent model
	_, err = suite.db.GetModel(suite.ctx, "non-existent-model")
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	// Test getting with empty ID
	_, err = suite.db.GetModel(suite.ctx, "")
	suite.Error(err)
	suite.Contains(err.Error(), "id not specified")
}

func (suite *ModelTestSuite) TestUpdateModel() {
	modelID := "test-update-model-" + system.GenerateAppID()
	model := &types.Model{
		ID:            modelID,
		Name:          "Update Test Model",
		Type:          types.ModelTypeChat,
		Runtime:       types.RuntimeOllama,
		ContextLength: 2048,
		Memory:        4 * 1024 * 1024 * 1024, // 4GB
		Description:   "Initial description",
		Enabled:       true,
	}

	// Create model first
	createdModel, err := suite.db.CreateModel(suite.ctx, model)
	suite.Require().NoError(err)
	initialUpdateTime := createdModel.Updated

	// Allow some time to pass to ensure Updated timestamp changes
	time.Sleep(10 * time.Millisecond)

	// Update fields
	updateData := &types.Model{
		ID:          modelID, // Must provide ID for update
		Name:        "Updated Name",
		Description: "Updated description",
		Enabled:     false,
		// Keep other fields same from 'model' to ensure they are persisted correctly
		Type:          model.Type,
		Runtime:       model.Runtime,
		ContextLength: model.ContextLength,
		Memory:        model.Memory,
		Created:       createdModel.Created, // Should be ignored by Save
	}

	updatedModel, err := suite.db.UpdateModel(suite.ctx, updateData)
	suite.NoError(err)
	suite.NotNil(updatedModel)
	suite.Equal(modelID, updatedModel.ID)
	suite.Equal("Updated Name", updatedModel.Name)
	suite.Equal("Updated description", updatedModel.Description)
	suite.Equal(false, updatedModel.Enabled)
	suite.Equal(model.Type, updatedModel.Type) // Ensure unchanged fields are persisted
	suite.Equal(model.Runtime, updatedModel.Runtime)
	suite.Equal(model.ContextLength, updatedModel.ContextLength)
	suite.Equal(model.Memory, updatedModel.Memory)
	suite.Equal(createdModel.Created, updatedModel.Created)   // Created should not change
	suite.True(updatedModel.Updated.After(initialUpdateTime)) // Updated should change

	// Test updating a non-existent model (GORM's Save behaves like Upsert if ID doesn't exist, but our UpdateModel requires ID)
	nonExistentUpdate := &types.Model{
		ID:   "non-existent-for-update",
		Name: "Non Existent",
	}
	_, err = suite.db.UpdateModel(suite.ctx, nonExistentUpdate)
	suite.Error(err) // Should fail because GetModel inside UpdateModel will return ErrNotFound

	// Test updating with empty ID
	emptyIDUpdate := &types.Model{
		Name: "Empty ID Update",
	}
	_, err = suite.db.UpdateModel(suite.ctx, emptyIDUpdate)
	suite.Error(err)
	suite.Contains(err.Error(), "id not specified")
}

func (suite *ModelTestSuite) TestListModels() {
	// Create some models with different properties
	model1 := &types.Model{ID: "list-model-1", Name: "List Model One", Type: types.ModelTypeChat, Runtime: types.RuntimeOllama, ContextLength: 1024, Memory: 1, Enabled: true}
	model2 := &types.Model{ID: "list-model-2", Name: "List Model Two", Type: types.ModelTypeChat, Runtime: types.RuntimeVLLM, ContextLength: 2048, Memory: 2, Enabled: false}
	model3 := &types.Model{ID: "list-model-3", Name: "List Model Three", Type: types.ModelTypeImage, Runtime: types.RuntimeDiffusers, ContextLength: 0, Memory: 3, Enabled: true}
	model4 := &types.Model{ID: "list-model-4", Name: "List Model Four", Type: types.ModelTypeChat, Runtime: types.RuntimeOllama, ContextLength: 4096, Memory: 4, Enabled: true}

	_, err := suite.db.CreateModel(suite.ctx, model1)
	suite.Require().NoError(err)
	_, err = suite.db.CreateModel(suite.ctx, model2)
	suite.Require().NoError(err)
	_, err = suite.db.CreateModel(suite.ctx, model3)
	suite.Require().NoError(err)
	_, err = suite.db.CreateModel(suite.ctx, model4)
	suite.Require().NoError(err)

	// Test listing all models (no filters) - includes seeded models if any
	allModels, err := suite.db.ListModels(suite.ctx, &ListModelsQuery{})
	suite.NoError(err)
	suite.GreaterOrEqual(len(allModels), 4, "Should list at least the 4 created models") // Use >= because seedModels might run

	// Test filtering by Type
	chatModels, err := suite.db.ListModels(suite.ctx, &ListModelsQuery{Type: types.ModelTypeChat})
	suite.NoError(err)
	suite.Len(chatModels, 3, "Should list 3 chat models")
	for _, m := range chatModels {
		suite.Equal(types.ModelTypeChat, m.Type)
	}

	imageModels, err := suite.db.ListModels(suite.ctx, &ListModelsQuery{Type: types.ModelTypeImage})
	suite.NoError(err)
	suite.Len(imageModels, 1, "Should list 1 image model")
	suite.Equal(types.ModelTypeImage, imageModels[0].Type)

	// Test filtering by Runtime
	ollamaModels, err := suite.db.ListModels(suite.ctx, &ListModelsQuery{Runtime: types.RuntimeOllama})
	suite.NoError(err)
	suite.Len(ollamaModels, 2, "Should list 2 ollama models")
	for _, m := range ollamaModels {
		suite.Equal(types.RuntimeOllama, m.Runtime)
	}

	// Test filtering by Enabled status
	enabled := true
	enabledModels, err := suite.db.ListModels(suite.ctx, &ListModelsQuery{Enabled: &enabled})
	suite.NoError(err)
	suite.Len(enabledModels, 3, "Should list 3 enabled models")
	for _, m := range enabledModels {
		suite.True(m.Enabled)
	}

	disabled := false
	disabledModels, err := suite.db.ListModels(suite.ctx, &ListModelsQuery{Enabled: &disabled})
	suite.NoError(err)
	suite.Len(disabledModels, 1, "Should list 1 disabled model")
	suite.False(disabledModels[0].Enabled)

	// Test filtering by Name (exact match)
	nameModels, err := suite.db.ListModels(suite.ctx, &ListModelsQuery{Name: "List Model One"})
	suite.NoError(err)
	suite.Len(nameModels, 1, "Should list 1 model by exact name")
	suite.Equal("List Model One", nameModels[0].Name)

	// Test combining filters
	enabledChatModels, err := suite.db.ListModels(suite.ctx, &ListModelsQuery{Type: types.ModelTypeChat, Enabled: &enabled})
	suite.NoError(err)
	suite.Len(enabledChatModels, 2, "Should list 2 enabled chat models")
	for _, m := range enabledChatModels {
		suite.True(m.Enabled)
		suite.Equal(types.ModelTypeChat, m.Type)
	}

	// Test filter resulting in no models
	noModels, err := suite.db.ListModels(suite.ctx, &ListModelsQuery{Name: "Non Existent Name"})
	suite.NoError(err)
	suite.Empty(noModels, "Should list no models for non-existent name")
}

func (suite *ModelTestSuite) TestDeleteModel() {
	modelID := "test-delete-model-" + system.GenerateAppID()
	model := &types.Model{
		ID:            modelID,
		Name:          "Delete Test Model",
		Type:          types.ModelTypeChat,
		Runtime:       types.RuntimeOllama,
		ContextLength: 1024,
		Memory:        1,
		Enabled:       true,
	}

	// Create model first
	_, err := suite.db.CreateModel(suite.ctx, model)
	suite.Require().NoError(err)

	// Verify it exists
	_, err = suite.db.GetModel(suite.ctx, modelID)
	suite.Require().NoError(err)

	// Delete the model
	err = suite.db.DeleteModel(suite.ctx, modelID)
	suite.NoError(err)

	// Verify it's gone
	_, err = suite.db.GetModel(suite.ctx, modelID)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	// Test deleting a non-existent model
	err = suite.db.DeleteModel(suite.ctx, "non-existent-delete")
	suite.NoError(err) // GORM delete doesn't return error for non-existent record by default

	// Test deleting with empty ID
	err = suite.db.DeleteModel(suite.ctx, "")
	suite.Error(err)
	suite.Contains(err.Error(), "id not specified")
}
