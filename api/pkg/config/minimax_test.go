package config

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestMiniMaxResolvedConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		cfg           MiniMax
		wantOpenAI    string
		wantAnthropic string
		wantSelected  string
	}{
		{
			name:          "global defaults",
			cfg:           MiniMax{},
			wantOpenAI:    types.MiniMaxGlobalOpenAIBaseURL,
			wantAnthropic: types.MiniMaxGlobalAnthropicBaseURL,
			wantSelected:  types.MiniMaxGlobalOpenAIBaseURL,
		},
		{
			name: "China Anthropic-compatible endpoint",
			cfg: MiniMax{
				Region:    "cn",
				APIFormat: types.ProviderAPIFormatAnthropic,
			},
			wantOpenAI:    types.MiniMaxCNOpenAIBaseURL,
			wantAnthropic: types.MiniMaxCNAnthropicBaseURL,
			wantSelected:  types.MiniMaxCNAnthropicBaseURL,
		},
		{
			name: "custom endpoints are normalized",
			cfg: MiniMax{
				APIFormat:        types.ProviderAPIFormatAnthropic,
				OpenAIBaseURL:    "https://openai.example/v1/",
				AnthropicBaseURL: "https://anthropic.example/api/",
			},
			wantOpenAI:    "https://openai.example/v1",
			wantAnthropic: "https://anthropic.example/api",
			wantSelected:  "https://anthropic.example/api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantOpenAI, tt.cfg.ResolvedOpenAIBaseURL())
			assert.Equal(t, tt.wantAnthropic, tt.cfg.ResolvedAnthropicBaseURL())
			assert.Equal(t, tt.wantSelected, tt.cfg.ResolvedBaseURL())
		})
	}

	assert.Equal(t, types.MiniMaxModels, (MiniMax{}).ResolvedModels())
}
