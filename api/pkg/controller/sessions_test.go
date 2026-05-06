package controller

import (
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
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

// -----------------------------------------------------------------------------
// getClient default-provider validation — fences the silent fall-through to
// "helix" when the configured default doesn't actually serve the requested
// model, so misroutes surface as "model X not in default provider" instead of
// the misleading "no runner has model X" downstream.
// -----------------------------------------------------------------------------

func (suite *ControllerSuite) Test_getClient_explicitProvider_skipsValidation() {
	// When a caller passes a non-empty provider, we don't second-guess. The
	// providerManager mock asserts client returned without ListModels being
	// touched (suite-wide GetClient mock returns a generic client; this test
	// just ensures no validation error is raised).
	suite.controller.Options.Config.Inference.Provider = "helix"

	client, err := suite.controller.getClient(suite.ctx, "", "user_x", "openai", "gpt-5.4")
	suite.NoError(err)
	suite.NotNil(client)
}

func (suite *ControllerSuite) Test_getClient_emptyModel_skipsValidation() {
	// model="" is the explicit opt-out (used by bootstrap paths where the
	// model isn't known yet). We must not call ListModels.
	suite.controller.Options.Config.Inference.Provider = "helix"

	client, err := suite.controller.getClient(suite.ctx, "", "user_x", "", "")
	suite.NoError(err)
	suite.NotNil(client)
}

func (suite *ControllerSuite) Test_getClient_defaultedProvider_modelServed_OK() {
	// provider=="" defaults to Inference.Provider ("helix"). When that
	// provider's ListModels includes the requested model, getClient succeeds.
	suite.controller.Options.Config.Inference.Provider = "helix"

	// The suite's blanket GetClient mock returns a generic openAIClient that
	// doesn't have ListModels stubbed. Replace it with a focused mock for
	// this specific case. We need to clear the existing AnyTimes stub by
	// setting up a new controller.
	ctrl := gomock.NewController(suite.T())
	pm := newMockProviderManagerForGetClientTest(ctrl, []types.OpenAIModel{
		{ID: "helix-model-x"},
	})
	suite.controller.providerManager = pm

	client, err := suite.controller.getClient(suite.ctx, "", "user_x", "", "helix-model-x")
	suite.NoError(err)
	suite.NotNil(client)
}

func (suite *ControllerSuite) Test_getClient_defaultedProvider_modelNotServed_returnsExplicitError() {
	// provider=="" defaults to Inference.Provider ("helix"). When that
	// provider's ListModels does NOT include the requested model, getClient
	// surfaces a clear routing error instead of letting the request fail
	// downstream as "no runner has model X".
	suite.controller.Options.Config.Inference.Provider = "helix"

	ctrl := gomock.NewController(suite.T())
	pm := newMockProviderManagerForGetClientTest(ctrl, []types.OpenAIModel{
		{ID: "some-other-model"},
	})
	suite.controller.providerManager = pm

	_, err := suite.controller.getClient(suite.ctx, "", "user_x", "", "gpt-5.4")
	suite.Require().Error(err)
	suite.Contains(err.Error(), `model "gpt-5.4" is not configured in the default provider "helix"`)
	suite.Contains(err.Error(), `prefix the model with the target provider`)
	suite.Contains(err.Error(), `"openai/gpt-5.4"`)
}


// newMockProviderManagerForGetClientTest builds a ProviderManager mock whose
// GetClient returns an openAI client whose ListModels returns the given list.
// Used by getClient defaulted-provider tests to stub the validation path.
func newMockProviderManagerForGetClientTest(ctrl *gomock.Controller, models []types.OpenAIModel) *manager.MockProviderManager {
	pm := manager.NewMockProviderManager(ctrl)
	c := oai.NewMockClient(ctrl)
	c.EXPECT().ListModels(gomock.Any()).Return(models, nil).AnyTimes()
	pm.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(c, nil).AnyTimes()
	return pm
}
