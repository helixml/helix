package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/types"
)

var errNoCodeAgentProviderContext = errors.New("session has no code-agent provider context")

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
	if attribution.hasExecutionConfig {
		return nil, fmt.Errorf("code-agent task has no API provider selected")
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
	return nil, errNoCodeAgentProviderContext
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

	ownerID := user.ID
	ownerType := types.OwnerTypeUser
	if user.OrganizationID != "" {
		ownerID = user.OrganizationID
		ownerType = types.OwnerTypeOrg
	}
	if types.IsGlobalProviderID(providerRef) {
		providerName := types.CanonicalProviderName(providerRef)
		var endpoint *types.ProviderEndpoint
		var err error
		if providerName == string(types.ProviderOpenAI) {
			endpoint, err = s.getBuiltInOpenAIProviderEndpoint()
		} else {
			endpoint, err = s.getBuiltInProviderEndpoint(ctx, providerName)
		}
		if err != nil {
			return nil, err
		}
		endpoint.ID = providerRef
		return endpoint, nil
	}
	if s.providerManager != nil {
		endpoints, err := s.providerManager.ListProviderEndpointsForOwner(ctx, ownerID, ownerType)
		if err != nil {
			return nil, fmt.Errorf("list providers: %w", err)
		}
		if endpoint := resolveProviderEndpointRef(endpoints, providerRef); endpoint != nil {
			// Provider-manager global entries are discovery-only synthetics and
			// intentionally do not expose credentials. Hydrate a selected built-in
			// before the code-agent proxy tries to authenticate upstream. A real
			// org/DB endpoint wins in resolveProviderEndpointRef and is left intact.
			if endpoint.ID == types.GlobalProviderID(string(types.ProviderOpenAI)) {
				return s.getBuiltInOpenAIProviderEndpoint()
			}
			if endpoint.ID == types.GlobalProviderID(string(types.ProviderAnthropic)) {
				return s.getBuiltInProviderEndpoint(ctx, string(types.ProviderAnthropic))
			}
			return endpoint, nil
		}
	}

	switch types.CanonicalProviderName(providerRef) {
	case string(types.ProviderOpenAI):
		return s.getBuiltInOpenAIProviderEndpoint()
	case string(types.ProviderAnthropic):
		return s.getBuiltInProviderEndpoint(ctx, string(types.ProviderAnthropic))
	}
	if s.providerManager == nil {
		return nil, fmt.Errorf("provider manager is not configured")
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
