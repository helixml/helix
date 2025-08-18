package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestDynamicModelInfoTestSuite(t *testing.T) {
	suite.Run(t, new(DynamicModelInfoTestSuite))
}

type DynamicModelInfoTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
}

func (suite *DynamicModelInfoTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.db = GetTestDB()

	// Clean up database before each test
	suite.Require().NoError(suite.db.gdb.Exec("DELETE FROM dynamic_model_infos").Error)
}

func (suite *DynamicModelInfoTestSuite) TearDownTestSuite() {
	// No need to close the database connection here as it's managed by TestMain
}

func (suite *DynamicModelInfoTestSuite) TestCreateDynamicModelInfo() {
	modelID := "test-dynamic-model-" + system.GenerateAppID()
	validModelInfo := &types.DynamicModelInfo{
		ID:       modelID,
		Provider: "helix",
		Name:     "Test Dynamic Model",
		ModelInfo: types.ModelInfo{
			ProviderSlug:        "helix",
			ProviderModelID:     "test-model-1",
			Slug:                "test-model",
			Name:                "Test Model",
			Author:              "Test Author",
			SupportedParameters: []string{"temperature", "max_tokens"},
			Description:         "A test dynamic model",
			InputModalities:     []types.Modality{types.ModalityText},
			OutputModalities:    []types.Modality{types.ModalityText},
			SupportsReasoning:   true,
			ContextLength:       4096,
			MaxCompletionTokens: 2048,
			Pricing: types.Pricing{
				Prompt:     "0.001",
				Completion: "0.002",
			},
		},
	}

	// Test creating a valid dynamic model info
	createdModelInfo, err := suite.db.CreateDynamicModelInfo(suite.ctx, validModelInfo)
	suite.NoError(err)
	suite.NotNil(createdModelInfo)
	suite.Equal(validModelInfo.ID, createdModelInfo.ID)
	suite.Equal(validModelInfo.Provider, createdModelInfo.Provider)
	suite.Equal(validModelInfo.Name, createdModelInfo.Name)
	suite.Equal(validModelInfo.ModelInfo.Name, createdModelInfo.ModelInfo.Name)
	suite.NotZero(createdModelInfo.Created)
	suite.NotZero(createdModelInfo.Updated)

	// Test creating a dynamic model info with missing ID
	invalidModelInfoNoID := &types.DynamicModelInfo{
		Provider: "helix",
		Name:     "Invalid Model",
	}
	_, err = suite.db.CreateDynamicModelInfo(suite.ctx, invalidModelInfoNoID)
	suite.Error(err)
	suite.Contains(err.Error(), "id not specified")

	// Test creating a dynamic model info with missing Provider
	invalidModelInfoNoProvider := &types.DynamicModelInfo{
		ID:   "invalid-model-no-provider",
		Name: "Invalid Model",
	}
	_, err = suite.db.CreateDynamicModelInfo(suite.ctx, invalidModelInfoNoProvider)
	suite.Error(err)
	suite.Contains(err.Error(), "provider not specified")

	// Test creating a dynamic model info with missing Name
	invalidModelInfoNoName := &types.DynamicModelInfo{
		ID:       "invalid-model-no-name",
		Provider: "helix",
	}
	_, err = suite.db.CreateDynamicModelInfo(suite.ctx, invalidModelInfoNoName)
	suite.Error(err)
	suite.Contains(err.Error(), "name not specified")
}

func (suite *DynamicModelInfoTestSuite) TestGetDynamicModelInfo() {
	modelID := "test-get-dynamic-model-" + system.GenerateAppID()
	validModelInfo := &types.DynamicModelInfo{
		ID:       modelID,
		Provider: "helix",
		Name:     "Test Get Dynamic Model",
		ModelInfo: types.ModelInfo{
			ProviderSlug:    "helix",
			ProviderModelID: "test-get-model-1",
			Slug:            "test-get-model",
			Name:            "Test Get Model",
		},
	}

	// Create a model info first
	createdModelInfo, err := suite.db.CreateDynamicModelInfo(suite.ctx, validModelInfo)
	suite.NoError(err)

	// Test getting the created model info
	retrievedModelInfo, err := suite.db.GetDynamicModelInfo(suite.ctx, createdModelInfo.ID)
	suite.NoError(err)
	suite.NotNil(retrievedModelInfo)
	suite.Equal(createdModelInfo.ID, retrievedModelInfo.ID)
	suite.Equal(createdModelInfo.Provider, retrievedModelInfo.Provider)
	suite.Equal(createdModelInfo.Name, retrievedModelInfo.Name)

	// Test getting a non-existent model info
	_, err = suite.db.GetDynamicModelInfo(suite.ctx, "non-existent-id")
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	// Test getting with empty ID
	_, err = suite.db.GetDynamicModelInfo(suite.ctx, "")
	suite.Error(err)
	suite.Contains(err.Error(), "id not specified")
}

func (suite *DynamicModelInfoTestSuite) TestUpdateDynamicModelInfo() {
	modelID := "test-update-dynamic-model-" + system.GenerateAppID()
	validModelInfo := &types.DynamicModelInfo{
		ID:       modelID,
		Provider: "helix",
		Name:     "Test Update Dynamic Model",
		ModelInfo: types.ModelInfo{
			ProviderSlug:    "helix",
			ProviderModelID: "test-update-model-1",
			Slug:            "test-update-model",
			Name:            "Test Update Model",
		},
	}

	// Create a model info first
	createdModelInfo, err := suite.db.CreateDynamicModelInfo(suite.ctx, validModelInfo)
	suite.NoError(err)

	// Test updating the model info
	updateTime := createdModelInfo.Updated
	time.Sleep(1 * time.Millisecond) // Ensure time difference

	createdModelInfo.Name = "Updated Dynamic Model Name"
	createdModelInfo.ModelInfo.Name = "Updated Model Name"
	updatedModelInfo, err := suite.db.UpdateDynamicModelInfo(suite.ctx, createdModelInfo)
	suite.NoError(err)
	suite.NotNil(updatedModelInfo)
	suite.Equal("Updated Dynamic Model Name", updatedModelInfo.Name)
	suite.Equal("Updated Model Name", updatedModelInfo.ModelInfo.Name)
	suite.True(updatedModelInfo.Updated.After(updateTime))

	// Test updating a non-existent model info
	nonExistentModelInfo := &types.DynamicModelInfo{
		ID:       "non-existent-id",
		Provider: "helix",
		Name:     "Non Existent Model",
	}
	_, err = suite.db.UpdateDynamicModelInfo(suite.ctx, nonExistentModelInfo)
	suite.Error(err)
	suite.True(errors.Is(err, ErrNotFound))

	// Test updating with empty ID
	invalidModelInfo := &types.DynamicModelInfo{
		Provider: "helix",
		Name:     "Invalid Model",
	}
	_, err = suite.db.UpdateDynamicModelInfo(suite.ctx, invalidModelInfo)
	suite.Error(err)
	suite.Contains(err.Error(), "id not specified")
}

func (suite *DynamicModelInfoTestSuite) TestListDynamicModelInfos() {
	// Create multiple model infos for testing
	modelInfo1 := &types.DynamicModelInfo{
		ID:       "test-list-model-1-" + system.GenerateAppID(),
		Provider: "helix",
		Name:     "Test List Model 1",
		ModelInfo: types.ModelInfo{
			ProviderSlug:    "helix",
			ProviderModelID: "test-list-model-1",
			Slug:            "test-list-model-1",
			Name:            "Test List Model 1",
		},
	}

	modelInfo2 := &types.DynamicModelInfo{
		ID:       "test-list-model-2-" + system.GenerateAppID(),
		Provider: "openai",
		Name:     "Test List Model 2",
		ModelInfo: types.ModelInfo{
			ProviderSlug:    "openai",
			ProviderModelID: "test-list-model-2",
			Slug:            "test-list-model-2",
			Name:            "Test List Model 2",
		},
	}

	modelInfo3 := &types.DynamicModelInfo{
		ID:       "test-list-model-3-" + system.GenerateAppID(),
		Provider: "helix",
		Name:     "Test List Model 3",
		ModelInfo: types.ModelInfo{
			ProviderSlug:    "helix",
			ProviderModelID: "test-list-model-3",
			Slug:            "test-list-model-3",
			Name:            "Test List Model 3",
		},
	}

	// Create all model infos
	_, err := suite.db.CreateDynamicModelInfo(suite.ctx, modelInfo1)
	suite.NoError(err)
	_, err = suite.db.CreateDynamicModelInfo(suite.ctx, modelInfo2)
	suite.NoError(err)
	_, err = suite.db.CreateDynamicModelInfo(suite.ctx, modelInfo3)
	suite.NoError(err)

	// Test listing all model infos
	allModelInfos, err := suite.db.ListDynamicModelInfos(suite.ctx, &types.ListDynamicModelInfosQuery{})
	suite.NoError(err)
	suite.Len(allModelInfos, 3)

	// Test filtering by provider
	helixModelInfos, err := suite.db.ListDynamicModelInfos(suite.ctx, &types.ListDynamicModelInfosQuery{
		Provider: "helix",
	})
	suite.NoError(err)
	suite.Len(helixModelInfos, 2)

	// Test filtering by name
	namedModelInfos, err := suite.db.ListDynamicModelInfos(suite.ctx, &types.ListDynamicModelInfosQuery{
		Name: "Test List Model 1",
	})
	suite.NoError(err)
	suite.Len(namedModelInfos, 1)
	suite.Equal("Test List Model 1", namedModelInfos[0].Name)

	// Test filtering by both provider and name
	specificModelInfos, err := suite.db.ListDynamicModelInfos(suite.ctx, &types.ListDynamicModelInfosQuery{
		Provider: "helix",
		Name:     "Test List Model 1",
	})
	suite.NoError(err)
	suite.Len(specificModelInfos, 1)
	suite.Equal("helix", specificModelInfos[0].Provider)
	suite.Equal("Test List Model 1", specificModelInfos[0].Name)
}

func (suite *DynamicModelInfoTestSuite) TestDeleteDynamicModelInfo() {
	modelID := "test-delete-dynamic-model-" + system.GenerateAppID()
	validModelInfo := &types.DynamicModelInfo{
		ID:       modelID,
		Provider: "helix",
		Name:     "Test Delete Dynamic Model",
		ModelInfo: types.ModelInfo{
			ProviderSlug:    "helix",
			ProviderModelID: "test-delete-model-1",
			Slug:            "test-delete-model",
			Name:            "Test Delete Model",
		},
	}

	// Create a model info first
	createdModelInfo, err := suite.db.CreateDynamicModelInfo(suite.ctx, validModelInfo)
	suite.NoError(err)

	// Verify it exists
	retrievedModelInfo, err := suite.db.GetDynamicModelInfo(suite.ctx, createdModelInfo.ID)
	suite.NoError(err)
	suite.NotNil(retrievedModelInfo)

	// Test deleting the model info
	err = suite.db.DeleteDynamicModelInfo(suite.ctx, createdModelInfo.ID)
	suite.NoError(err)

	// Verify it's deleted
	_, err = suite.db.GetDynamicModelInfo(suite.ctx, createdModelInfo.ID)
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	// Test deleting a non-existent model info
	err = suite.db.DeleteDynamicModelInfo(suite.ctx, "non-existent-id")
	suite.NoError(err) // GORM doesn't return error for deleting non-existent records

	// Test deleting with empty ID
	err = suite.db.DeleteDynamicModelInfo(suite.ctx, "")
	suite.Error(err)
	suite.Contains(err.Error(), "id not specified")
}
