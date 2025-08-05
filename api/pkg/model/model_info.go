package model

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/helixml/helix/api/pkg/types"
)

//go:embed model_info.json
var modelInfo embed.FS

type ModelInfoProvider interface { //nolint:revive
	GetModelInfo(ctx context.Context, request *ModelInfoRequest) (*types.ModelInfo, error)
}

type ModelInfoRequest struct { //nolint:revive
	Slug     string
	Provider string
	Model    string
}

type BaseModelInfoProvider struct {
	dataMu *sync.RWMutex
	data   map[string]types.ModelInfo // Keyed by provider/model slug
}

func NewBaseModelInfoProvider() (*BaseModelInfoProvider, error) {
	jsonFile, err := modelInfo.ReadFile("model_info.json")
	if err != nil {
		return nil, err
	}

	var response ModelInfoResponse
	err = json.Unmarshal(jsonFile, &response)
	if err != nil {
		return nil, err
	}

	data := make(map[string]types.ModelInfo)
	for _, model := range response.Data {
		data[model.Slug] = types.ModelInfo{
			Name:                model.Name,
			Author:              model.Author,
			Description:         model.Description,
			InputModalities:     model.InputModalities,
			OutputModalities:    model.OutputModalities,
			SupportsReasoning:   model.Endpoint.SupportsReasoning,
			ContextLength:       model.ContextLength,
			MaxCompletionTokens: model.Endpoint.MaxCompletionTokens,
			Pricing:             model.Endpoint.Pricing,
		}
	}

	return &BaseModelInfoProvider{
		dataMu: &sync.RWMutex{},
		data:   data,
	}, nil
}

func (p *BaseModelInfoProvider) GetModelInfo(_ context.Context, request *ModelInfoRequest) (*types.ModelInfo, error) {
	p.dataMu.RLock()
	defer p.dataMu.RUnlock()

	if request.Slug != "" {
		modelInfo, ok := p.data[request.Slug]
		if !ok {
			return nil, fmt.Errorf("model info not found for slug: %s", request.Slug)
		}

		return &modelInfo, nil
	}

	// Otherwise lookup by provider
	slug := fmt.Sprintf("%s/%s", request.Provider, request.Model)
	modelInfo, ok := p.data[slug]
	if ok {
		return &modelInfo, nil
	}

	// Fallback to just model
	for _, model := range p.data {
		if model.Name == request.Model {
			return &model, nil
		}
	}

	return nil, fmt.Errorf("model info not found for model: %s", request.Model)
}
