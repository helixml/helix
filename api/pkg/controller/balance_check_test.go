package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func testControllerForBalanceCheck(mockStore *store.MockStore, billingEnabled bool, minimumInferenceBalance float64) *Controller {
	return &Controller{
		Options: Options{
			Config: &config.ServerConfig{
				Stripe: config.Stripe{
					BillingEnabled:          billingEnabled,
					MinimumInferenceBalance: minimumInferenceBalance,
				},
			},
			Store: mockStore,
		},
	}
}

func TestHasEnoughBalance(t *testing.T) {
	t.Run("runner token skips balance check", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		c := testControllerForBalanceCheck(mockStore, true, 0.01)

		ok, err := c.HasEnoughBalance(context.Background(), &types.User{
			ID:         "user-1",
			TokenType:  types.TokenTypeRunner,
			Waitlisted: true,
		}, "", true)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("waitlisted user is rejected", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		c := testControllerForBalanceCheck(mockStore, true, 0.01)

		ok, err := c.HasEnoughBalance(context.Background(), &types.User{
			ID:         "user-1",
			TokenType:  types.TokenTypeSession,
			Waitlisted: true,
		}, "", true)
		require.Error(t, err)
		assert.EqualError(t, err, "user is waitlisted")
		assert.False(t, ok)
	})

	t.Run("billing disabled globally allows requests", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		c := testControllerForBalanceCheck(mockStore, false, 0.01)

		ok, err := c.HasEnoughBalance(context.Background(), &types.User{
			ID:        "user-1",
			TokenType: types.TokenTypeSession,
		}, "", true)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("client billing disabled allows requests", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		c := testControllerForBalanceCheck(mockStore, true, 0.01)

		ok, err := c.HasEnoughBalance(context.Background(), &types.User{
			ID:        "user-1",
			TokenType: types.TokenTypeSession,
		}, "", false)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("returns error when fetching org wallet fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		c := testControllerForBalanceCheck(mockStore, true, 0.01)
		mockStore.EXPECT().GetWalletByOrg(gomock.Any(), "org-1").Return(nil, errors.New("db down"))

		ok, err := c.HasEnoughBalance(context.Background(), &types.User{
			ID:        "user-1",
			TokenType: types.TokenTypeSession,
		}, "org-1", true)
		require.Error(t, err)
		assert.EqualError(t, err, "failed to get wallet: db down")
		assert.False(t, ok)
	})

	t.Run("returns error when fetching user wallet fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		c := testControllerForBalanceCheck(mockStore, true, 0.01)
		mockStore.EXPECT().GetWalletByUser(gomock.Any(), "user-1").Return(nil, errors.New("db down"))

		ok, err := c.HasEnoughBalance(context.Background(), &types.User{
			ID:        "user-1",
			TokenType: types.TokenTypeSession,
		}, "", true)
		require.Error(t, err)
		assert.EqualError(t, err, "failed to get wallet: db down")
		assert.False(t, ok)
	})

	t.Run("returns false when balance is below minimum", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		c := testControllerForBalanceCheck(mockStore, true, 1.0)
		mockStore.EXPECT().GetWalletByUser(gomock.Any(), "user-1").Return(&types.Wallet{
			UserID:  "user-1",
			Balance: 0.99,
		}, nil)

		ok, err := c.HasEnoughBalance(context.Background(), &types.User{
			ID:        "user-1",
			TokenType: types.TokenTypeSession,
		}, "", true)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("returns true when balance meets minimum", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		c := testControllerForBalanceCheck(mockStore, true, 1.0)
		mockStore.EXPECT().GetWalletByUser(gomock.Any(), "user-1").Return(&types.Wallet{
			UserID:  "user-1",
			Balance: 1.0,
		}, nil)

		ok, err := c.HasEnoughBalance(context.Background(), &types.User{
			ID:        "user-1",
			TokenType: types.TokenTypeSession,
		}, "", true)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("uses org wallet when org id is provided", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		c := testControllerForBalanceCheck(mockStore, true, 1.0)
		mockStore.EXPECT().GetWalletByOrg(gomock.Any(), "org-1").Return(&types.Wallet{
			OrgID:   "org-1",
			Balance: 2.0,
		}, nil)

		ok, err := c.HasEnoughBalance(context.Background(), &types.User{
			ID:        "user-1",
			TokenType: types.TokenTypeSession,
		}, "org-1", true)
		require.NoError(t, err)
		assert.True(t, ok)
	})
}
