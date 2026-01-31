package model

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	gomock "go.uber.org/mock/gomock"
)

func TestGetDynamicPricing(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := NewMockstore(ctrl)

	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	d := NewDynamicModelInfoProvider(store, b)

	store.EXPECT().ListDynamicModelInfos(gomock.Any(), &types.ListDynamicModelInfosQuery{
		Provider: "google",
		Name:     "models/gemini-2.0-flash-001",
	}).Return([]*types.DynamicModelInfo{}, nil)

	modelInfo, err := d.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "google",
		Model:    "models/gemini-2.0-flash-001",
	})
	assert.NoError(t, err)

	assert.Equal(t, "Google: Gemini 2.0 Flash", modelInfo.Name)
	assert.Equal(t, "0.0000004", modelInfo.Pricing.Completion)
}

func TestGetDynamicPricing_WithOverride(t *testing.T) {
	ctrl := gomock.NewController(t)
	store := NewMockstore(ctrl)

	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	d := NewDynamicModelInfoProvider(store, b)

	store.EXPECT().ListDynamicModelInfos(gomock.Any(), &types.ListDynamicModelInfosQuery{
		Provider: "google",
		Name:     "models/gemini-2.0-flash-001",
	}).Return([]*types.DynamicModelInfo{
		{
			ModelInfo: types.ModelInfo{
				Pricing: types.Pricing{
					Completion: "0.0000005",
				},
			},
		},
	}, nil)

	modelInfo, err := d.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "google",
		Model:    "models/gemini-2.0-flash-001",
	})
	assert.NoError(t, err)

	assert.Equal(t, "Google: Gemini 2.0 Flash", modelInfo.Name)
	assert.Equal(t, "0.0000005", modelInfo.Pricing.Completion)
}
