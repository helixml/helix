package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/types"
)

func TestParseDynamicProviders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []DynamicProviderConfig
		wantErr  bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
			wantErr:  false,
		},
		{
			name:  "single provider with base URL",
			input: "groq:gsk_xxxx:https://api.groq.com/openai/v1",
			expected: []DynamicProviderConfig{
				{
					Name:    "groq",
					APIKey:  "gsk_xxxx",
					BaseURL: "https://api.groq.com/openai/v1",
				},
			},
			wantErr: false,
		},
		{
			name:  "single provider without base URL",
			input: "openai:sk_xxxx",
			expected: []DynamicProviderConfig{
				{
					Name:    "openai",
					APIKey:  "sk_xxxx",
					BaseURL: "",
				},
			},
			wantErr: false,
		},
		{
			name:  "multiple providers",
			input: "groq:gsk_xxxx:https://api.groq.com/openai/v1,cerebras:csk_yyyy:https://api.cerebras.ai/v1,openai:sk_zzzz",
			expected: []DynamicProviderConfig{
				{
					Name:    "groq",
					APIKey:  "gsk_xxxx",
					BaseURL: "https://api.groq.com/openai/v1",
				},
				{
					Name:    "cerebras",
					APIKey:  "csk_yyyy",
					BaseURL: "https://api.cerebras.ai/v1",
				},
				{
					Name:    "openai",
					APIKey:  "sk_zzzz",
					BaseURL: "",
				},
			},
			wantErr: false,
		},
		{
			name:     "invalid format - missing API key",
			input:    "provider1",
			expected: nil,
			wantErr:  true,
		},
		{
			name:  "URL with colons",
			input: "provider1:key:url:extra",
			expected: []DynamicProviderConfig{
				{
					Name:    "provider1",
					APIKey:  "key",
					BaseURL: "url:extra",
				},
			},
			wantErr: false,
		},
		{
			name:     "invalid format - empty provider name",
			input:    ":key:url",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid format - empty API key",
			input:    "provider1::url",
			expected: nil,
			wantErr:  true,
		},
		{
			name:  "whitespace handling",
			input: " groq : gsk_xxxx : https://api.groq.com/openai/v1 , cerebras : csk_yyyy : https://api.cerebras.ai/v1 ",
			expected: []DynamicProviderConfig{
				{
					Name:    "groq",
					APIKey:  "gsk_xxxx",
					BaseURL: "https://api.groq.com/openai/v1",
				},
				{
					Name:    "cerebras",
					APIKey:  "csk_yyyy",
					BaseURL: "https://api.cerebras.ai/v1",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDynamicProviders(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestToProviderEndpoint(t *testing.T) {
	config := DynamicProviderConfig{
		Name:    "test-provider",
		APIKey:  "test-key",
		BaseURL: "https://api.test.com/v1",
	}

	endpoint := config.ToProviderEndpoint()

	assert.Equal(t, "test-provider", endpoint.Name)
	assert.Equal(t, "Dynamic provider: test-provider", endpoint.Description)
	assert.Equal(t, "https://api.test.com/v1", endpoint.BaseURL)
	assert.Equal(t, "test-key", endpoint.APIKey)
	assert.Equal(t, types.ProviderEndpointTypeGlobal, endpoint.EndpointType)
	assert.Equal(t, string(types.OwnerTypeSystem), endpoint.Owner)
	assert.Equal(t, types.OwnerTypeSystem, endpoint.OwnerType)
}
