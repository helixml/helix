package server

import (
	"context"
	"fmt"
	"os"
	"strings"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *HelixAPIServer) codeAgentProviderSelection(
	ctx context.Context,
	user *types.User,
) (*codeAgentProviderSelection, error) {
	attribution, err := s.resolveProxyUsageAttribution(ctx, user, "")
	if err != nil {
		return nil, err
	}
	if attribution.CodeAgentOverrides != nil && attribution.CodeAgentOverrides.ProviderRef != "" {
		return &codeAgentProviderSelection{
			Attribution: attribution,
			Runtime:     attribution.CodeAgentRuntime,
			ProviderRef: attribution.CodeAgentOverrides.ProviderRef,
			Model:       attribution.CodeAgentOverrides.Model,
		}, nil
	}
	if attribution.AppID != "" {
		app, err := s.Store.GetApp(ctx, attribution.AppID)
		if err != nil {
			return nil, fmt.Errorf("load code-agent app: %w", err)
		}
		assistant := external_agent.FindZedExternalAssistant(app)
		provider, model := external_agent.AssistantModelSelection(assistant)
		if assistant != nil && provider != "" {
			return &codeAgentProviderSelection{
				Attribution: attribution,
				Runtime:     assistant.CodeAgentRuntime,
				ProviderRef: provider,
				Model:       model,
			}, nil
		}
	}
	return nil, fmt.Errorf("code-agent task has no API provider selected")
}

type codeAgentProviderSelection struct {
	Attribution *proxyUsageAttribution
	Runtime     types.CodeAgentRuntime
	ProviderRef string
	Model       string
}

func (s *HelixAPIServer) resolveCodeAgentProviderEndpoint(
	ctx context.Context,
	user *types.User,
	providerRef string,
) (*types.ProviderEndpoint, error) {
	if providerRef == "" {
		return nil, fmt.Errorf("provider reference is required")
	}

	if !strings.HasPrefix(providerRef, "pe_") {
		switch {
		case strings.EqualFold(providerRef, string(types.ProviderOpenAI)):
			return s.getBuiltInOpenAIProviderEndpoint()
		case strings.EqualFold(providerRef, string(types.ProviderAnthropic)):
			return s.getBuiltInProviderEndpoint(ctx, string(types.ProviderAnthropic))
		}
	}

	if s.providerManager == nil {
		return nil, fmt.Errorf("provider manager is not configured")
	}
	ownerID := user.ID
	ownerType := types.OwnerTypeUser
	if user.OrganizationID != "" {
		ownerID = user.OrganizationID
		ownerType = types.OwnerTypeOrg
	}
	endpoints, err := s.providerManager.ListProviderEndpointsForOwner(ctx, ownerID, ownerType)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	for _, endpoint := range endpoints {
		if endpoint.ID == providerRef || strings.EqualFold(endpoint.Name, providerRef) {
			if endpoint.ID == "" {
				return s.resolveCodeAgentProviderEndpoint(ctx, user, endpoint.Name)
			}
			return endpoint, nil
		}
	}
	return nil, fmt.Errorf("provider %q is not available to this task", providerRef)
}

func (s *HelixAPIServer) getBuiltInOpenAIProviderEndpoint() (*types.ProviderEndpoint, error) {
	apiKey := s.Cfg.Providers.OpenAI.APIKey
	if apiKey == "" && s.Cfg.Providers.OpenAI.APIKeyFromFile != "" {
		data, err := os.ReadFile(s.Cfg.Providers.OpenAI.APIKeyFromFile)
		if err != nil {
			return nil, fmt.Errorf("read OpenAI API key file: %w", err)
		}
		apiKey = strings.TrimSpace(string(data))
	}
	if apiKey == "" {
		return nil, fmt.Errorf("openai provider is not configured")
	}
	return &types.ProviderEndpoint{
		ID:             string(types.ProviderOpenAI),
		Name:           string(types.ProviderOpenAI),
		BaseURL:        s.Cfg.Providers.OpenAI.BaseURL,
		APIKey:         apiKey,
		EndpointType:   types.ProviderEndpointTypeGlobal,
		Owner:          string(types.OwnerTypeSystem),
		OwnerType:      types.OwnerTypeSystem,
		BillingEnabled: s.Cfg.Providers.BillingEnabled,
	}, nil
}

func providerEndpointAPIKey(endpoint *types.ProviderEndpoint) (string, error) {
	if endpoint.APIKeyFromFile == "" {
		if endpoint.APIKey == "" {
			return "", fmt.Errorf("provider %q has no API key", endpoint.Name)
		}
		return endpoint.APIKey, nil
	}
	data, err := os.ReadFile(endpoint.APIKeyFromFile)
	if err != nil {
		return "", fmt.Errorf("read provider API key file: %w", err)
	}
	apiKey := strings.TrimSpace(string(data))
	if apiKey == "" {
		return "", fmt.Errorf("provider %q API key file is empty", endpoint.Name)
	}
	return apiKey, nil
}
