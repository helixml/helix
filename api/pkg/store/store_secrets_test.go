package store

import (
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *PostgresStoreTestSuite) TestSecretOperations() {
	// Test Create Secret
	suite.Run("CreateSecret", func() {
		secret := &types.Secret{
			Name:  "test-secret",
			Owner: "test-owner-" + system.GenerateUUID(),
			Value: []byte("test-value"),
		}

		createdSecret, err := suite.db.CreateSecret(suite.ctx, secret)
		require.NoError(suite.T(), err)
		assert.NotEmpty(suite.T(), createdSecret.ID)
		assert.Equal(suite.T(), secret.Name, createdSecret.Name)
		assert.Equal(suite.T(), secret.Owner, createdSecret.Owner)
		assert.Equal(suite.T(), secret.Value, createdSecret.Value)

		// Clean up
		suite.T().Cleanup(func() {
			err := suite.db.DeleteSecret(suite.ctx, createdSecret.ID)
			assert.NoError(suite.T(), err)
		})
	})

	// Test List Secrets
	suite.Run("ListSecrets", func() {
		owner := "test-owner-" + system.GenerateUUID()
		secrets := []*types.Secret{
			{Name: "secret1", Owner: owner, Value: []byte("value1")},
			{Name: "secret2", Owner: owner, Value: []byte("value2")},
			{Name: "secret3", Owner: owner, Value: []byte("value3")},
		}

		for _, s := range secrets {
			_, err := suite.db.CreateSecret(suite.ctx, s)
			require.NoError(suite.T(), err)
		}

		listedSecrets, err := suite.db.ListSecrets(suite.ctx, &ListSecretsQuery{
			Owner: owner,
		})
		require.NoError(suite.T(), err)
		assert.Len(suite.T(), listedSecrets, len(secrets))

		// Clean up
		suite.T().Cleanup(func() {
			for _, s := range listedSecrets {
				err := suite.db.DeleteSecret(suite.ctx, s.ID)
				assert.NoError(suite.T(), err)
			}
		})
	})

	// Test Update Secret
	suite.Run("UpdateSecret", func() {
		secret := &types.Secret{
			Name:  "update-test-secret",
			Owner: "test-owner-" + system.GenerateUUID(),
			Value: []byte("original-value"),
		}

		createdSecret, err := suite.db.CreateSecret(suite.ctx, secret)
		require.NoError(suite.T(), err)

		updatedSecret := &types.Secret{
			ID:    createdSecret.ID,
			Name:  "updated-secret",
			Owner: createdSecret.Owner,
			Value: []byte("updated-value"),
		}

		updatedSecret, err = suite.db.UpdateSecret(suite.ctx, updatedSecret)
		require.NoError(suite.T(), err)

		fetchedSecret, err := suite.db.GetSecret(suite.ctx, createdSecret.ID)
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), updatedSecret.Name, fetchedSecret.Name)
		assert.Equal(suite.T(), updatedSecret.Value, fetchedSecret.Value)

		// Clean up
		suite.T().Cleanup(func() {
			err := suite.db.DeleteSecret(suite.ctx, createdSecret.ID)
			assert.NoError(suite.T(), err)
		})
	})

	// Test Delete Secret
	suite.Run("DeleteSecret", func() {
		secret := &types.Secret{
			Name:  "delete-test-secret",
			Owner: "test-owner-" + system.GenerateUUID(),
			Value: []byte("delete-me"),
		}

		createdSecret, err := suite.db.CreateSecret(suite.ctx, secret)
		require.NoError(suite.T(), err)

		err = suite.db.DeleteSecret(suite.ctx, createdSecret.ID)
		require.NoError(suite.T(), err)

		// Verify the secret is deleted
		_, err = suite.db.GetSecret(suite.ctx, createdSecret.ID)
		assert.Error(suite.T(), err)
	})
}
