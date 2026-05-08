package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

// TestDecideKoditEmbedding documents the opt-in contract: external embedding
// providers are only selected when an admin has explicitly configured both a
// provider AND a model in System Settings. In every other shape of settings
// (including nil, empty, and half-configured) Kodit must fall back to its
// built-in local models so a fresh install boots without any upstream
// dependency.
func TestDecideKoditEmbedding(t *testing.T) {
	cases := []struct {
		name       string
		settings   *types.SystemSettings
		wantText   bool
		wantVision bool
	}{
		{
			name:       "nil settings — fresh install, built-in models",
			settings:   nil,
			wantText:   false,
			wantVision: false,
		},
		{
			name:       "empty settings — admin has not configured anything",
			settings:   &types.SystemSettings{},
			wantText:   false,
			wantVision: false,
		},
		{
			name: "only text provider set, no model — half-configured, treat as unset",
			settings: &types.SystemSettings{
				KoditTextEmbeddingProvider: "qwen-text-embedding",
			},
			wantText:   false,
			wantVision: false,
		},
		{
			name: "only text model set, no provider — half-configured, treat as unset",
			settings: &types.SystemSettings{
				KoditTextEmbeddingModel: "Qwen/Qwen3-Embedding-8B",
			},
			wantText:   false,
			wantVision: false,
		},
		{
			name: "text fully configured, vision unset — text external, vision local",
			settings: &types.SystemSettings{
				KoditTextEmbeddingProvider: "qwen-text-embedding",
				KoditTextEmbeddingModel:    "Qwen/Qwen3-Embedding-8B",
			},
			wantText:   true,
			wantVision: false,
		},
		{
			name: "vision fully configured, text unset — text local, vision external",
			settings: &types.SystemSettings{
				KoditVisionEmbeddingProvider: "qwen-vision-embedding",
				KoditVisionEmbeddingModel:    "Qwen/Qwen3-VL-Embedding-8B",
			},
			wantText:   false,
			wantVision: true,
		},
		{
			name: "both fully configured — both external",
			settings: &types.SystemSettings{
				KoditTextEmbeddingProvider:   "qwen-text-embedding",
				KoditTextEmbeddingModel:      "Qwen/Qwen3-Embedding-8B",
				KoditVisionEmbeddingProvider: "qwen-vision-embedding",
				KoditVisionEmbeddingModel:    "Qwen/Qwen3-VL-Embedding-8B",
			},
			wantText:   true,
			wantVision: true,
		},
		{
			name: "vision half-configured, text fully configured — text external, vision local",
			settings: &types.SystemSettings{
				KoditTextEmbeddingProvider:   "qwen-text-embedding",
				KoditTextEmbeddingModel:      "Qwen/Qwen3-Embedding-8B",
				KoditVisionEmbeddingProvider: "qwen-vision-embedding",
				// KoditVisionEmbeddingModel missing
			},
			wantText:   true,
			wantVision: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decideKoditEmbedding(tc.settings)
			assert.Equal(t, tc.wantText, got.UseExternalText, "UseExternalText")
			assert.Equal(t, tc.wantVision, got.UseExternalVision, "UseExternalVision")
		})
	}
}
