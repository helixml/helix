package store

import (
	"testing"

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
	anotherOwner := "another-owner-" + system.GenerateUUID()

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
		{
			Name:         "endpoint4",
			Owner:        anotherOwner,
			EndpointType: types.ProviderEndpointTypeUser,
			BaseURL:      "https://api4.example.com",
			APIKey:       "key4",
		},
	}

	for _, e := range endpoints {
		created, err := suite.db.CreateProviderEndpoint(suite.ctx, e)
		require.NoError(suite.T(), err)

		suite.T().Cleanup(func() {
			err := suite.db.DeleteProviderEndpoint(suite.ctx, created.ID)
			assert.NoError(suite.T(), err)
		})
	}

	// Test listing all endpoints for owner
	listedEndpoints, err := suite.db.ListProviderEndpoints(suite.ctx, &ListProviderEndpointsQuery{
		Owner:      owner,
		WithGlobal: true,
	})
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), listedEndpoints, 3, "should list all user endpoints and a global one")

	suite.T().Run("UserEndpoints", func(t *testing.T) {
		// Test listing endpoints for user only
		userEndpoints, err := suite.db.ListProviderEndpoints(suite.ctx, &ListProviderEndpointsQuery{
			Owner:      owner,
			OwnerType:  types.OwnerTypeUser,
			WithGlobal: false,
		})
		require.NoError(t, err)
		assert.Len(t, userEndpoints, 2, "should list user endpoints only (endpoint1 and endpoint3)")
	})

	// Test listing for another owner
	anotherOwnerEndpoints, err := suite.db.ListProviderEndpoints(suite.ctx, &ListProviderEndpointsQuery{
		Owner:      anotherOwner,
		WithGlobal: false,
	})
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), anotherOwnerEndpoints, 1)
}

func (suite *PostgresStoreTestSuite) TestProviderEndpointListWithGlobalFromOtherOwner() {
	// Verify that WithGlobal returns global endpoints created by other users.
	// This is the core use case: an admin creates a global provider endpoint,
	// and all users can see it regardless of who owns it.
	adminOwner := "admin-" + system.GenerateUUID()
	userOwner := "user-" + system.GenerateUUID()

	endpoints := []*types.ProviderEndpoint{
		{
			Name:         "admin-global-ollama",
			Owner:        adminOwner,
			EndpointType: types.ProviderEndpointTypeGlobal,
			BaseURL:      "http://ollama:11434",
			APIKey:       "key1",
		},
		{
			Name:         "user-private",
			Owner:        userOwner,
			EndpointType: types.ProviderEndpointTypeUser,
			BaseURL:      "https://my-provider.example.com",
			APIKey:       "key2",
		},
	}

	for _, e := range endpoints {
		created, err := suite.db.CreateProviderEndpoint(suite.ctx, e)
		require.NoError(suite.T(), err)
		suite.T().Cleanup(func() {
			err := suite.db.DeleteProviderEndpoint(suite.ctx, created.ID)
			assert.NoError(suite.T(), err)
		})
	}

	// User should see their own user endpoint + admin's global endpoint
	listed, err := suite.db.ListProviderEndpoints(suite.ctx, &ListProviderEndpointsQuery{
		Owner:      userOwner,
		WithGlobal: true,
	})
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), listed, 2, "should see own user endpoint + admin's global endpoint")

	names := make(map[string]bool)
	for _, ep := range listed {
		names[ep.Name] = true
	}
	assert.True(suite.T(), names["user-private"], "should include user's own endpoint")
	assert.True(suite.T(), names["admin-global-ollama"], "should include admin's global endpoint")

	// Without WithGlobal, user should only see their own endpoint
	userOnly, err := suite.db.ListProviderEndpoints(suite.ctx, &ListProviderEndpointsQuery{
		Owner:      userOwner,
		WithGlobal: false,
	})
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), userOnly, 1, "should only see own user endpoint")
	assert.Equal(suite.T(), "user-private", userOnly[0].Name)
}

func (suite *PostgresStoreTestSuite) TestProviderEndpointListOrgScoped() {
	// Regression test: org-scoped endpoints (endpoint_type='org') must be
	// returned when the query specifies OwnerType=org. Previously the store
	// hardcoded the ownership filter to 'user' and org endpoints were invisible.
	userOwner := "user-" + system.GenerateUUID()
	orgOwner := "org-" + system.GenerateUUID()
	adminOwner := "admin-" + system.GenerateUUID()

	endpoints := []*types.ProviderEndpoint{
		{
			Name:         "user-endpoint",
			Owner:        userOwner,
			OwnerType:    types.OwnerTypeUser,
			EndpointType: types.ProviderEndpointTypeUser,
			BaseURL:      "https://user.example.com",
			APIKey:       "user-key",
		},
		{
			Name:         "org-endpoint",
			Owner:        orgOwner,
			OwnerType:    types.OwnerTypeOrg,
			EndpointType: types.ProviderEndpointTypeOrg,
			BaseURL:      "https://org.example.com",
			APIKey:       "org-key",
		},
		{
			Name:         "global-endpoint",
			Owner:        adminOwner,
			EndpointType: types.ProviderEndpointTypeGlobal,
			BaseURL:      "https://global.example.com",
			APIKey:       "global-key",
		},
	}

	for _, e := range endpoints {
		created, err := suite.db.CreateProviderEndpoint(suite.ctx, e)
		require.NoError(suite.T(), err)
		suite.T().Cleanup(func() {
			err := suite.db.DeleteProviderEndpoint(suite.ctx, created.ID)
			assert.NoError(suite.T(), err)
		})
	}

	suite.T().Run("OrgQueryReturnsOrgPlusGlobal", func(t *testing.T) {
		listed, err := suite.db.ListProviderEndpoints(suite.ctx, &ListProviderEndpointsQuery{
			Owner:      orgOwner,
			OwnerType:  types.OwnerTypeOrg,
			WithGlobal: true,
		})
		require.NoError(t, err)

		names := make(map[string]bool)
		for _, ep := range listed {
			names[ep.Name] = true
		}
		assert.True(t, names["org-endpoint"], "org query should include the org-owned endpoint")
		assert.True(t, names["global-endpoint"], "org query should include the global endpoint")
		assert.False(t, names["user-endpoint"], "org query must not include the user-owned endpoint")
		assert.Len(t, listed, 2, "org query should return exactly the org + global endpoints")
	})

	suite.T().Run("UserQueryReturnsUserPlusGlobal", func(t *testing.T) {
		listed, err := suite.db.ListProviderEndpoints(suite.ctx, &ListProviderEndpointsQuery{
			Owner:      userOwner,
			OwnerType:  types.OwnerTypeUser,
			WithGlobal: true,
		})
		require.NoError(t, err)

		names := make(map[string]bool)
		for _, ep := range listed {
			names[ep.Name] = true
		}
		assert.True(t, names["user-endpoint"], "user query should include the user-owned endpoint")
		assert.True(t, names["global-endpoint"], "user query should include the global endpoint")
		assert.False(t, names["org-endpoint"], "user query must not include the org-owned endpoint")
		assert.Len(t, listed, 2, "user query should return exactly the user + global endpoints")
	})

	suite.T().Run("OrgQueryWithoutGlobalReturnsOrgOnly", func(t *testing.T) {
		listed, err := suite.db.ListProviderEndpoints(suite.ctx, &ListProviderEndpointsQuery{
			Owner:      orgOwner,
			OwnerType:  types.OwnerTypeOrg,
			WithGlobal: false,
		})
		require.NoError(t, err)
		require.Len(t, listed, 1, "org query without global should return only the org-owned endpoint")
		assert.Equal(t, "org-endpoint", listed[0].Name)
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

	fetchedEndpoint, err := suite.db.GetProviderEndpoint(suite.ctx, &GetProviderEndpointsQuery{ID: createdEndpoint.ID})
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
	_, err = suite.db.GetProviderEndpoint(suite.ctx, &GetProviderEndpointsQuery{ID: createdEndpoint.ID})
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
