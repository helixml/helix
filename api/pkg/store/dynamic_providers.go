package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

// DynamicProviderConfig represents a single dynamic provider configuration
type DynamicProviderConfig struct {
	Name    string
	APIKey  string
	BaseURL string
}

// ParseDynamicProviders parses the DYNAMIC_PROVIDERS environment variable
// Expected format: "provider1:api_key1:base_url1,provider2:api_key2:base_url2"
// Returns a slice of DynamicProviderConfig structs
func ParseDynamicProviders(dynamicProviders string) ([]DynamicProviderConfig, error) {
	if dynamicProviders == "" {
		return nil, nil
	}

	var configs []DynamicProviderConfig

	// Split by comma to get individual provider configurations
	providerSpecs := strings.Split(dynamicProviders, ",")

	for _, spec := range providerSpecs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}

		// Split by colon to get name:key:url
		// We need to be careful with URLs that contain colons (like https://)
		// So we'll only split on the first two colons to separate name:key from the rest
		firstColon := strings.Index(spec, ":")
		if firstColon == -1 {
			return nil, fmt.Errorf("invalid provider specification '%s': expected format 'name:api_key' or 'name:api_key:base_url'", spec)
		}

		name := strings.TrimSpace(spec[:firstColon])
		remainder := strings.TrimSpace(spec[firstColon+1:])

		if name == "" {
			return nil, fmt.Errorf("provider name cannot be empty in specification '%s'", spec)
		}

		// Find the second colon for the API key
		secondColon := strings.Index(remainder, ":")
		var apiKey, baseURL string

		if secondColon == -1 {
			// No base URL provided, just name:api_key
			apiKey = remainder
		} else {
			// Base URL provided, name:api_key:base_url
			apiKey = strings.TrimSpace(remainder[:secondColon])
			baseURL = strings.TrimSpace(remainder[secondColon+1:])
		}

		if apiKey == "" {
			return nil, fmt.Errorf("API key cannot be empty for provider '%s'", name)
		}

		config := DynamicProviderConfig{
			Name:    name,
			APIKey:  apiKey,
			BaseURL: baseURL,
		}

		configs = append(configs, config)
	}

	return configs, nil
}

// ToProviderEndpoint converts a DynamicProviderConfig to a ProviderEndpoint
func (d *DynamicProviderConfig) ToProviderEndpoint() *types.ProviderEndpoint {
	return &types.ProviderEndpoint{
		Name:         d.Name,
		Description:  fmt.Sprintf("Dynamic provider: %s", d.Name),
		BaseURL:      d.BaseURL,
		APIKey:       d.APIKey,
		EndpointType: types.ProviderEndpointTypeGlobal,
		Owner:        string(types.OwnerTypeSystem),
		OwnerType:    types.OwnerTypeSystem,
	}
}

// InitializeDynamicProviders creates new provider endpoints from the DYNAMIC_PROVIDERS environment variable.
// It will only create providers that don't already exist in the database to avoid overwriting manual configurations.
func (s *PostgresStore) InitializeDynamicProviders(ctx context.Context, dynamicProviders string) error {
	if dynamicProviders == "" {
		log.Debug().Msg("No dynamic providers configured")
		return nil
	}

	log.Info().
		Str("dynamic_providers", dynamicProviders).
		Msg("Initializing dynamic providers")

	// Parse the dynamic providers string
	configs, err := ParseDynamicProviders(dynamicProviders)
	if err != nil {
		return fmt.Errorf("failed to parse dynamic providers: %w", err)
	}

	log.Info().
		Int("provider_count", len(configs)).
		Msg("Parsed dynamic provider configurations")

	// Get existing provider endpoints to check for duplicates
	existingEndpoints, err := s.ListProviderEndpoints(ctx, &ListProviderEndpointsQuery{
		WithGlobal: true,
	})
	if err != nil {
		return fmt.Errorf("failed to list existing provider endpoints: %w", err)
	}

	// Create a map of existing provider names for quick lookup
	existingNames := make(map[string]*types.ProviderEndpoint)
	for _, endpoint := range existingEndpoints {
		// Only consider system-owned global endpoints for dynamic provider updates
		if endpoint.OwnerType == types.OwnerTypeSystem && endpoint.EndpointType == types.ProviderEndpointTypeGlobal {
			existingNames[endpoint.Name] = endpoint
		}
	}

	// Process each dynamic provider configuration
	for _, config := range configs {
		log.Info().
			Str("provider_name", config.Name).
			Str("base_url", config.BaseURL).
			Msg("Processing dynamic provider")

		endpoint := config.ToProviderEndpoint()

		// Check if provider already exists
		if _, exists := existingNames[config.Name]; exists {
			// Skip existing providers to avoid overwriting manual configurations
			log.Info().
				Str("provider_name", config.Name).
				Msg("Provider already exists in database, skipping to avoid overwriting existing configuration")
			continue
		}

		// Create new endpoint
		log.Info().
			Str("provider_name", config.Name).
			Msg("Creating new dynamic provider endpoint")

		_, err := s.CreateProviderEndpoint(ctx, endpoint)
		if err != nil {
			log.Error().
				Err(err).
				Str("provider_name", config.Name).
				Msg("Failed to create dynamic provider endpoint")
			continue
		}

		log.Info().
			Str("provider_name", config.Name).
			Msg("Successfully created dynamic provider endpoint")
	}

	log.Info().
		Int("total_configured", len(configs)).
		Msg("Finished processing dynamic providers (existing providers were skipped)")

	return nil
}

// InitializeBuiltInProviders creates provider endpoints for built-in providers when their
// API keys are set via environment variables (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.).
// This allows these providers to be used by the LLM proxy endpoints without manual configuration.
func (s *PostgresStore) InitializeBuiltInProviders(ctx context.Context, providers *config.Providers) error {
	if providers == nil {
		return nil
	}

	// Get existing provider endpoints to check for duplicates
	existingEndpoints, err := s.ListProviderEndpoints(ctx, &ListProviderEndpointsQuery{
		WithGlobal: true,
	})
	if err != nil {
		return fmt.Errorf("failed to list existing provider endpoints: %w", err)
	}

	// Create a map of existing provider names for quick lookup
	existingNames := make(map[string]*types.ProviderEndpoint)
	for _, endpoint := range existingEndpoints {
		if endpoint.OwnerType == types.OwnerTypeSystem && endpoint.EndpointType == types.ProviderEndpointTypeGlobal {
			existingNames[endpoint.Name] = endpoint
		}
	}

	// Define built-in provider configurations
	builtInProviders := []struct {
		name    string
		apiKey  string
		baseURL string
	}{
		{
			name:    string(types.ProviderAnthropic),
			apiKey:  providers.Anthropic.APIKey,
			baseURL: providers.Anthropic.BaseURL,
		},
		{
			name:    string(types.ProviderOpenAI),
			apiKey:  providers.OpenAI.APIKey,
			baseURL: providers.OpenAI.BaseURL,
		},
		{
			name:    string(types.ProviderTogetherAI),
			apiKey:  providers.TogetherAI.APIKey,
			baseURL: providers.TogetherAI.BaseURL,
		},
	}

	for _, provider := range builtInProviders {
		if provider.apiKey == "" {
			continue
		}

		// Skip if provider already exists
		if _, exists := existingNames[provider.name]; exists {
			log.Debug().
				Str("provider_name", provider.name).
				Msg("Built-in provider already exists in database, skipping")
			continue
		}

		log.Info().
			Str("provider_name", provider.name).
			Str("base_url", provider.baseURL).
			Msg("Creating built-in provider endpoint from environment variable")

		endpoint := &types.ProviderEndpoint{
			Name:           provider.name,
			Description:    fmt.Sprintf("Built-in %s provider (auto-configured from environment)", provider.name),
			BaseURL:        provider.baseURL,
			APIKey:         provider.apiKey,
			EndpointType:   types.ProviderEndpointTypeGlobal,
			Owner:          string(types.OwnerTypeSystem),
			OwnerType:      types.OwnerTypeSystem,
			BillingEnabled: true,
		}

		_, err := s.CreateProviderEndpoint(ctx, endpoint)
		if err != nil {
			log.Error().
				Err(err).
				Str("provider_name", provider.name).
				Msg("Failed to create built-in provider endpoint")
			continue
		}

		log.Info().
			Str("provider_name", provider.name).
			Msg("Successfully created built-in provider endpoint")
	}

	return nil
}
