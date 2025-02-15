package store

import (
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *PostgresStoreTestSuite) TestProviderEndpointCreate() {
	endpoint := &types.ProviderEndpoint{
		Name:         "test-endpoint",
		Owner:        "test-owner-" + system.GenerateUUID(),
		EndpointType: types.ProviderEndpointTypeUser,
		BaseURL:      "https://api.example.com",
		APIKey:       "test-api-key",
	}

	createdEndpoint, err := suite.db.CreateProviderEndpoint(suite.ctx, endpoint)
	require.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), createdEndpoint.ID)
	assert.Equal(suite.T(), endpoint.Name, createdEndpoint.Name)
	assert.Equal(suite.T(), endpoint.Owner, createdEndpoint.Owner)
	assert.Equal(suite.T(), endpoint.EndpointType, createdEndpoint.EndpointType)
	assert.Equal(suite.T(), endpoint.BaseURL, createdEndpoint.BaseURL)
	assert.Equal(suite.T(), endpoint.APIKey, createdEndpoint.APIKey)
	assert.NotZero(suite.T(), createdEndpoint.Created)

	// Clean up
	suite.T().Cleanup(func() {
		err := suite.db.DeleteProviderEndpoint(suite.ctx, createdEndpoint.ID)
		assert.NoError(suite.T(), err)
	})
}

func (suite *PostgresStoreTestSuite) TestProviderEndpointList() {
	owner := "test-owner-" + system.GenerateUUID()
	endpoints := []*types.ProviderEndpoint{
		{
			Name:         "endpoint1",
			Owner:        owner,
			EndpointType: types.ProviderEndpointTypeUser,
			BaseURL:      "https://api1.example.com",
			APIKey:       "key1",
		},
		{
			Name:         "endpoint2",
			Owner:        owner,
			EndpointType: types.ProviderEndpointTypeGlobal,
			BaseURL:      "https://api2.example.com",
			APIKey:       "key2",
		},
		{
			Name:         "endpoint3",
			Owner:        owner,
			EndpointType: types.ProviderEndpointTypeUser,
			BaseURL:      "https://api3.example.com",
			APIKey:       "key3",
		},
	}

	for _, e := range endpoints {
		_, err := suite.db.CreateProviderEndpoint(suite.ctx, e)
		require.NoError(suite.T(), err)
	}

	// Test listing all endpoints for owner
	listedEndpoints, err := suite.db.ListProviderEndpoints(suite.ctx, &ListProviderEndpointsQuery{
		Owner: owner,
	})
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), listedEndpoints, len(endpoints))

	// Test listing endpoints by type
	userEndpoints, err := suite.db.ListProviderEndpoints(suite.ctx, &ListProviderEndpointsQuery{
		Owner:        owner,
		EndpointType: types.ProviderEndpointTypeUser,
	})
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), userEndpoints, 2)

	// Clean up
	suite.T().Cleanup(func() {
		for _, e := range listedEndpoints {
			err := suite.db.DeleteProviderEndpoint(suite.ctx, e.ID)
			assert.NoError(suite.T(), err)
		}
	})
}

func (suite *PostgresStoreTestSuite) TestProviderEndpointUpdate() {
	endpoint := &types.ProviderEndpoint{
		Name:         "update-test-endpoint",
		Owner:        "test-owner-" + system.GenerateUUID(),
		EndpointType: types.ProviderEndpointTypeUser,
		BaseURL:      "https://api.example.com",
		APIKey:       "original-key",
	}

	createdEndpoint, err := suite.db.CreateProviderEndpoint(suite.ctx, endpoint)
	require.NoError(suite.T(), err)

	updatedEndpoint := &types.ProviderEndpoint{
		ID:           createdEndpoint.ID,
		Name:         "updated-endpoint",
		Owner:        createdEndpoint.Owner,
		EndpointType: types.ProviderEndpointTypeGlobal,
		BaseURL:      "https://updated.example.com",
		APIKey:       "updated-key",
	}

	updatedEndpoint, err = suite.db.UpdateProviderEndpoint(suite.ctx, updatedEndpoint)
	require.NoError(suite.T(), err)

	fetchedEndpoint, err := suite.db.GetProviderEndpoint(suite.ctx, createdEndpoint.ID)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), updatedEndpoint.Name, fetchedEndpoint.Name)
	assert.Equal(suite.T(), updatedEndpoint.EndpointType, fetchedEndpoint.EndpointType)
	assert.Equal(suite.T(), updatedEndpoint.BaseURL, fetchedEndpoint.BaseURL)
	assert.Equal(suite.T(), updatedEndpoint.APIKey, fetchedEndpoint.APIKey)
	assert.NotZero(suite.T(), fetchedEndpoint.Updated)

	// Clean up
	suite.T().Cleanup(func() {
		err := suite.db.DeleteProviderEndpoint(suite.ctx, createdEndpoint.ID)
		assert.NoError(suite.T(), err)
	})
}

func (suite *PostgresStoreTestSuite) TestProviderEndpointDelete() {
	endpoint := &types.ProviderEndpoint{
		Name:         "delete-test-endpoint",
		Owner:        "test-owner-" + system.GenerateUUID(),
		EndpointType: types.ProviderEndpointTypeUser,
		BaseURL:      "https://api.example.com",
		APIKey:       "delete-me",
	}

	createdEndpoint, err := suite.db.CreateProviderEndpoint(suite.ctx, endpoint)
	require.NoError(suite.T(), err)

	err = suite.db.DeleteProviderEndpoint(suite.ctx, createdEndpoint.ID)
	require.NoError(suite.T(), err)

	// Verify the endpoint is deleted
	_, err = suite.db.GetProviderEndpoint(suite.ctx, createdEndpoint.ID)
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrNotFound, err)
}

func (suite *PostgresStoreTestSuite) TestProviderEndpointCreateValidation() {
	// Test missing owner
	endpoint := &types.ProviderEndpoint{
		Name:         "test-endpoint",
		EndpointType: types.ProviderEndpointTypeUser,
		BaseURL:      "https://api.example.com",
		APIKey:       "test-api-key",
	}

	_, err := suite.db.CreateProviderEndpoint(suite.ctx, endpoint)
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "owner not specified")
}

func (suite *PostgresStoreTestSuite) TestProviderEndpointUpdateValidation() {
	// Test missing ID
	endpoint := &types.ProviderEndpoint{
		Name:         "test-endpoint",
		Owner:        "test-owner",
		EndpointType: types.ProviderEndpointTypeUser,
		BaseURL:      "https://api.example.com",
		APIKey:       "test-api-key",
	}

	_, err := suite.db.UpdateProviderEndpoint(suite.ctx, endpoint)
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "id not specified")

	// Test missing owner
	endpoint.ID = "test-id"
	endpoint.Owner = ""
	_, err = suite.db.UpdateProviderEndpoint(suite.ctx, endpoint)
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "owner not specified")
}
