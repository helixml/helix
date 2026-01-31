package controller

import (
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

func (suite *ControllerSuite) Test_checkInferenceTokenQuota_LimitReached_FreeSubscription_WithinLimit() {
	suite.controller.Options.Config.SubscriptionQuotas.Enabled = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Enabled = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Pro.MaxMonthlyTokens = 100000
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Pro.Strict = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Free.MaxMonthlyTokens = 1000
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Free.Strict = true

	defer func() {
		suite.controller.Options.Config.SubscriptionQuotas.Enabled = false
	}()

	userID := "test-user-id"

	suite.store.EXPECT().GetUserMonthlyTokenUsage(suite.ctx, gomock.Any(), gomock.Any()).Return(100, nil)
	suite.store.EXPECT().GetUserMeta(suite.ctx, userID).Return(&types.UserMeta{
		Config: types.UserConfig{
			StripeSubscriptionActive: false,
		},
	}, nil)

	err := suite.controller.checkInferenceTokenQuota(suite.ctx, userID, "openai")
	suite.NoError(err)
}

func (suite *ControllerSuite) Test_checkInferenceTokenQuota_LimitReached_FreeSubscription_AboveLimit() {
	suite.controller.Options.Config.SubscriptionQuotas.Enabled = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Enabled = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Pro.MaxMonthlyTokens = 100000
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Pro.Strict = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Free.MaxMonthlyTokens = 1000
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Free.Strict = true

	defer func() {
		suite.controller.Options.Config.SubscriptionQuotas.Enabled = false
	}()

	userID := "test-user-id"

	suite.store.EXPECT().GetUserMonthlyTokenUsage(suite.ctx, gomock.Any(), gomock.Any()).Return(1000000, nil)
	suite.store.EXPECT().GetUserMeta(suite.ctx, userID).Return(&types.UserMeta{
		Config: types.UserConfig{
			StripeSubscriptionActive: false,
		},
	}, nil)

	err := suite.controller.checkInferenceTokenQuota(suite.ctx, userID, "openai")
	suite.Error(err)
	suite.Contains(err.Error(), "monthly token limit exceeded")
}

func (suite *ControllerSuite) Test_checkInferenceTokenQuota_LimitReached_FreeSubscription_AboveLimit_TheirOwnProvider() {
	suite.controller.Options.Config.SubscriptionQuotas.Enabled = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Enabled = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Pro.MaxMonthlyTokens = 100000
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Pro.Strict = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Free.MaxMonthlyTokens = 1000
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Free.Strict = true

	defer func() {
		suite.controller.Options.Config.SubscriptionQuotas.Enabled = false
	}()

	userID := "test-user-id"

	// No need to check for monthly usage as we are using their own provider

	err := suite.controller.checkInferenceTokenQuota(suite.ctx, userID, "my-own-provider")
	suite.NoError(err)
}

func (suite *ControllerSuite) Test_checkInferenceTokenQuota_LimitReached_ActiveSubscription_WithinLimit() {
	suite.controller.Options.Config.SubscriptionQuotas.Enabled = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Enabled = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Pro.MaxMonthlyTokens = 100000
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Pro.Strict = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Free.MaxMonthlyTokens = 1000
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Free.Strict = true

	defer func() {
		suite.controller.Options.Config.SubscriptionQuotas.Enabled = false
	}()

	userID := "test-user-id"

	suite.store.EXPECT().GetUserMonthlyTokenUsage(suite.ctx, gomock.Any(), gomock.Any()).Return(1000, nil)
	suite.store.EXPECT().GetUserMeta(suite.ctx, userID).Return(&types.UserMeta{
		Config: types.UserConfig{
			StripeSubscriptionActive: true,
		},
	}, nil)

	err := suite.controller.checkInferenceTokenQuota(suite.ctx, userID, "openai")
	suite.NoError(err)
}

func (suite *ControllerSuite) Test_checkInferenceTokenQuota_LimitReached_ActiveSubscription_AboveLimit() {
	suite.controller.Options.Config.SubscriptionQuotas.Enabled = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Enabled = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Pro.MaxMonthlyTokens = 100000
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Pro.Strict = true
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Free.MaxMonthlyTokens = 1000
	suite.controller.Options.Config.SubscriptionQuotas.Inference.Free.Strict = true

	defer func() {
		suite.controller.Options.Config.SubscriptionQuotas.Enabled = false
	}()

	userID := "test-user-id"

	suite.store.EXPECT().GetUserMonthlyTokenUsage(suite.ctx, gomock.Any(), gomock.Any()).Return(1000000, nil)
	suite.store.EXPECT().GetUserMeta(suite.ctx, userID).Return(&types.UserMeta{
		Config: types.UserConfig{
			StripeSubscriptionActive: true,
		},
	}, nil)

	err := suite.controller.checkInferenceTokenQuota(suite.ctx, userID, "openai")
	suite.Error(err)
	suite.Contains(err.Error(), "monthly token limit exceeded")
}
