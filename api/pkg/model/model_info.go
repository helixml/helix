package model

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/helixml/helix/api/pkg/types"
)

//go:embed model_info.json
var modelInfo embed.FS

type ModelInfoProvider interface { //nolint:revive
	GetModelInfo(ctx context.Context, request *ModelInfoRequest) (*types.ModelInfo, error)
}

type ModelInfoRequest struct { //nolint:revive
	BaseURL  string
	Provider string
	Model    string
}

type BaseModelInfoProvider struct {
	dataMu    *sync.RWMutex
	data      map[string]types.ModelInfo // Keyed by provider model ID
	providers map[string]string          // Keyed by provider base URL
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

	providers := make(map[string]string)

	data := make(map[string]types.ModelInfo)
	for _, model := range response.Data {
		data[model.Endpoint.ProviderModelID] = types.ModelInfo{
			ProviderSlug:        model.Endpoint.ProviderSlug,
			ProviderModelID:     model.Endpoint.ProviderModelID,
			Name:                model.Name,
			Slug:                model.Slug,
			Author:              model.Author,
			Description:         model.Description,
			InputModalities:     model.InputModalities,
			OutputModalities:    model.OutputModalities,
			SupportsReasoning:   model.Endpoint.SupportsReasoning,
			ContextLength:       model.ContextLength,
			SupportedParameters: model.Endpoint.SupportedParameters,
			MaxCompletionTokens: model.Endpoint.MaxCompletionTokens,
			Pricing:             model.Endpoint.Pricing,
		}

		providers[model.Endpoint.ProviderInfo.BaseURL] = model.Endpoint.ProviderInfo.Slug
	}

	return &BaseModelInfoProvider{
		dataMu:    &sync.RWMutex{},
		data:      data,
		providers: providers,
	}, nil
}

func (p *BaseModelInfoProvider) GetModelInfo(_ context.Context, request *ModelInfoRequest) (*types.ModelInfo, error) {
	p.dataMu.RLock()
	defer p.dataMu.RUnlock()

	// Try to get directly
	modelInfo, ok := p.data[request.Model]
	if ok {
		return &modelInfo, nil
	}

	// If it has "<prefix>/" strip it as we will be looking up by model name
	if strings.Contains(request.Model, "/") {
		// Strip the prefix
		parts := strings.SplitN(request.Model, "/", 2)
		request.Model = parts[1]
	}

	// Try again
	modelInfo, ok = p.data[request.Model]
	if ok {
		return &modelInfo, nil
	}

	provider, ok := p.getProvider(request.BaseURL)
	if !ok {
		provider = request.Provider
	}

	slug := fmt.Sprintf("%s/%s", provider, request.Model)

	// Lookup by slug or model name
	for _, model := range p.data {
		if model.Name == request.Model {
			return &model, nil
		}

		if model.Slug == slug {
			return &model, nil
		}

		// Check if the provider URL matches
	}

	return nil, fmt.Errorf("model info not found for model: %s (%s)", request.Model, slug)
}

func (p *BaseModelInfoProvider) getProvider(baseURL string) (string, bool) {
	if baseURL == "" {
		return "", false
	}

	provider, ok := p.providers[baseURL]
	if ok {
		return provider, true
	}

	// If it's google, remove the /openai suffix
	baseURL = strings.TrimSuffix(baseURL, "/openai")

	provider, ok = p.providers[baseURL]
	if ok {
		return provider, true
	}

	return "", false
}
