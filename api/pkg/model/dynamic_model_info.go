package model

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

//go:generate mockgen -source $GOFILE -destination dynamic_model_info_mocks.go -package $GOPACKAGE

type store interface {
	ListDynamicModelInfos(ctx context.Context, q *types.ListDynamicModelInfosQuery) ([]*types.DynamicModelInfo, error)
}

type DynamicModelInfoProvider struct {
	store store
	base  *BaseModelInfoProvider
}

func NewDynamicModelInfoProvider(store store, base *BaseModelInfoProvider) *DynamicModelInfoProvider {
	return &DynamicModelInfoProvider{
		store: store,
		base:  base,
	}
}

func (p *DynamicModelInfoProvider) GetModelInfo(ctx context.Context, request *ModelInfoRequest) (*types.ModelInfo, error) {
	baseModelInfo, err := p.base.GetModelInfo(ctx, request)

	dynamicModelInfos, dynamicErr := p.store.ListDynamicModelInfos(ctx, &types.ListDynamicModelInfosQuery{
		Provider: request.Provider,
		Name:     request.Model,
	})
	if dynamicErr != nil {
		return nil, fmt.Errorf("failed to list dynamic model infos: %w", dynamicErr)
	}

	switch {
	case baseModelInfo == nil && len(dynamicModelInfos) == 0:
		// Initial base info error
		return nil, err
	case baseModelInfo == nil && len(dynamicModelInfos) > 0:
		// We only have dynamic info defined, return it
		return &dynamicModelInfos[0].ModelInfo, nil
	default:
		// We have base model info and potentially dynamic, merge them
		return mergeModelInfo(baseModelInfo, dynamicModelInfos), nil
	}
}

func mergeModelInfo(baseModelInfo *types.ModelInfo, dynamicModelInfos []*types.DynamicModelInfo) *types.ModelInfo {
	modelInfo := *baseModelInfo

	if len(dynamicModelInfos) > 0 {
		modelInfo.Pricing = dynamicModelInfos[0].ModelInfo.Pricing
	}

	return &modelInfo
}
