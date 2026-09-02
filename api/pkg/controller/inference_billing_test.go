package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestValidateBillingModelPricing(t *testing.T) {
	tests := []struct {
		name           string
		billingEnabled bool
		modelInfo      *types.ModelInfo
		lookupErr      error
		wantErr        string
	}{
		{name: "billing disabled"},
		{name: "missing metadata", billingEnabled: true, lookupErr: errors.New("not found"), wantErr: "billing requires pricing metadata"},
		{name: "unpriced model", billingEnabled: true, modelInfo: &types.ModelInfo{}, wantErr: "billing requires non-zero pricing metadata"},
		{
			name:           "priced model",
			billingEnabled: true,
			modelInfo: &types.ModelInfo{Pricing: types.Pricing{
				Prompt:     "0.000002",
				Completion: "0.00001",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			modelInfoProvider := model.NewMockModelInfoProvider(ctrl)
			if tt.billingEnabled {
				modelInfoProvider.EXPECT().
					GetModelInfo(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, req *model.ModelInfoRequest) (*types.ModelInfo, error) {
						require.Equal(t, "anthropic", req.Provider)
						require.Equal(t, "claude-fable-5", req.Model)
						return tt.modelInfo, tt.lookupErr
					})
			}

			c := &Controller{Options: Options{ModelInfoProvider: modelInfoProvider}}
			err := c.validateBillingModelPricing(context.Background(), tt.billingEnabled, "anthropic", "claude-fable-5")
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestChatCompletionEntryPointsRejectMissingBillingPricing(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "blocking"
		if stream {
			name = "streaming"
		}
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := oai.NewMockClient(ctrl)
			client.EXPECT().BillingEnabled().Return(true)

			providerManager := manager.NewMockProviderManager(ctrl)
			providerManager.EXPECT().
				GetClient(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, req *manager.GetClientRequest) (oai.Client, error) {
					require.Equal(t, "anthropic", req.Provider)
					return client, nil
				})

			modelInfoProvider := model.NewMockModelInfoProvider(ctrl)
			modelInfoProvider.EXPECT().
				GetModelInfo(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, req *model.ModelInfoRequest) (*types.ModelInfo, error) {
					require.Equal(t, "anthropic", req.Provider)
					require.Equal(t, "claude-unpriced-test-model", req.Model)
					return nil, errors.New("not found")
				})

			c := &Controller{
				Options: Options{
					Config:            &config.ServerConfig{},
					ModelInfoProvider: modelInfoProvider,
				},
				providerManager: providerManager,
			}
			user := &types.User{ID: "user-1"}
			req := openai.ChatCompletionRequest{Model: "claude-unpriced-test-model"}
			opts := &ChatCompletionOptions{
				Provider:         "anthropic",
				CodeAgentRuntime: types.CodeAgentRuntimeOpenCode,
			}

			if stream {
				_, _, err := c.ChatCompletionStream(context.Background(), user, req, opts)
				require.ErrorContains(t, err, "billing requires pricing metadata")
				return
			}

			_, _, err := c.ChatCompletion(context.Background(), user, req, opts)
			require.ErrorContains(t, err, "billing requires pricing metadata")
		})
	}
}
