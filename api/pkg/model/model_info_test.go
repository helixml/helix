package model

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetGeminiFlash(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "google",
		Model:    "models/gemini-2.0-flash-001",
	})
	assert.NoError(t, err)

	assert.Equal(t, "Google: Gemini 2.0 Flash", modelInfo.Name)
	assert.Equal(t, "0.0000004", modelInfo.Pricing.Completion)
}

func TestToModelInfo_ReasoningEffortCapability(t *testing.T) {
	withEffort := ModelInfoData{
		ReasoningConfig: &ReasoningConfig{SupportsReasoningEffort: true},
	}
	withEffort.Endpoint.SupportsReasoning = true

	assert.True(t, toModelInfo(withEffort).SupportsReasoningEffort)
	assert.False(t, toModelInfo(ModelInfoData{}).SupportsReasoningEffort)
}

func Test_GetQwen38VisionModalities(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	require.NoError(t, err)

	for _, modelID := range []string{"qwen3.8-27b", "qwen/qwen3.8-27b"} {
		t.Run(modelID, func(t *testing.T) {
			modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
				Provider: "openrouter",
				Model:    modelID,
			})
			require.NoError(t, err)

			assert.Equal(t, []types.Modality{
				types.ModalityText,
				types.ModalityImage,
				types.Modality("video"),
			}, modelInfo.InputModalities)
			assert.Equal(t, []types.Modality{types.ModalityText}, modelInfo.OutputModalities)
			assert.Equal(t, 1_000_000, modelInfo.ContextLength)
			assert.Equal(t, 131_072, modelInfo.MaxCompletionTokens)
			assert.Equal(t, "0.0000004", modelInfo.Pricing.Prompt)
			assert.Equal(t, "0.000003", modelInfo.Pricing.Completion)
		})
	}
}

func Test_GetHaiku35(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "anthropic",
		// Model:    "claude-3-5-haiku-20241022",
		Model: "anthropic/claude-3.5-haiku",
	})
	assert.NoError(t, err)

	assert.Equal(t, "Anthropic: Claude 3.5 Haiku", modelInfo.Name)
	assert.Equal(t, "0.000004", modelInfo.Pricing.Completion)

	// With date
	modelInfo, err = b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "anthropic",
		Model:    "claude-3-5-haiku-20241022",
	})
	assert.NoError(t, err)

	assert.Equal(t, "Anthropic: Claude 3.5 Haiku", modelInfo.Name)
	assert.Equal(t, "0.000004", modelInfo.Pricing.Completion)
}

func Test_GetOpus45(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "anthropic",
		Model:    "anthropic/claude-opus-4.5",
	})
	assert.NoError(t, err)

	assert.Equal(t, "Anthropic: Claude Opus 4.5", modelInfo.Name)
	assert.Equal(t, "0.000025", modelInfo.Pricing.Completion)
}

func Test_GetOpus46(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "anthropic",
		Model:    "anthropic/claude-opus-4.6",
	})
	assert.NoError(t, err)

	assert.Equal(t, "Anthropic: Claude Opus 4.6", modelInfo.Name)
	assert.Equal(t, "0.000025", modelInfo.Pricing.Completion)
}

func Test_GetGptOSS20B(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "openai",
		Model:    "gpt-oss-20b",
	})
	assert.NoError(t, err)

	assert.Equal(t, "OpenAI: gpt-oss-20b", modelInfo.Name)
	assert.Equal(t, "0.00000014", modelInfo.Pricing.Completion)
}

func Test_GetGeminiFlash_CustomUserProvider(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "pv_123",
		Model:    "google/gemini-2.5-flash",
		BaseURL:  "https://generativelanguage.googleapis.com/v1beta/openai",
	})
	assert.NoError(t, err)

	assert.Equal(t, "Google: Gemini 2.5 Flash", modelInfo.Name)
	assert.Equal(t, "0.0000025", modelInfo.Pricing.Completion)
}

func Test_GetOpenAIo3Mini(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "openai",
		Model:    "o3-mini",
	})
	require.NoError(t, err)

	assert.Equal(t, "OpenAI: o3 Mini", modelInfo.Name)
	assert.Equal(t, "0.0000044", modelInfo.Pricing.Completion)
}

func Test_GetOpenAIo3Mini_CustomUserProvider(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "id_123",
		BaseURL:  "https://api.openai.com/v1",
		Model:    "o3-mini",
	})
	assert.NoError(t, err)

	assert.Equal(t, "OpenAI: o3 Mini", modelInfo.Name)
	assert.Equal(t, "0.0000044", modelInfo.Pricing.Completion)
}

func Test_GetClaudeSonnet4(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "anthropic",
		Model:    "claude-sonnet-4",
	})
	require.NoError(t, err)

	assert.Equal(t, "Anthropic: Claude Sonnet 4", modelInfo.Name)
	assert.Equal(t, "0.000015", modelInfo.Pricing.Completion)
}

func Test_GetClaudeSonnet4_5(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "anthropic",
		Model:    "claude-sonnet-4.5",
	})
	require.NoError(t, err)

	assert.Equal(t, "Anthropic: Claude Sonnet 4.5", modelInfo.Name)
	assert.Equal(t, "0.000015", modelInfo.Pricing.Completion)
}

func Test_GetClaudeSonnet4_CustomUserProvider(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	assert.NoError(t, err)

	modelInfo, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "id_123",
		Model:    "claude-sonnet-4",
		BaseURL:  "https://api.anthropic.com",
	})
	require.NoError(t, err)

	assert.Equal(t, "Anthropic: Claude Sonnet 4", modelInfo.Name)
	assert.Equal(t, "0.000015", modelInfo.Pricing.Completion)
}

// Test_NormalizeModelID locks in the id-normalization rules so the
// fallback lookup keeps catching ids that share a normalized form.
func Test_NormalizeModelID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Plain Anthropic ids stay put.
		{"claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"claude-opus-4-7", "claude-opus-4-7"},
		// Bedrock region prefixes get stripped.
		{"eu.anthropic.claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"us.anthropic.claude-opus-4-1-20250805-v1:0", "claude-opus-4-1-20250805"},
		{"global.anthropic.claude-opus-4-6-v1", "claude-opus-4-6"},
		{"apac.anthropic.claude-haiku-4-5-20251001-v1", "claude-haiku-4-5-20251001"},
		// Bedrock version suffixes.
		{"claude-opus-4-7-v1:0", "claude-opus-4-7"},
		{"claude-opus-4-7-v1", "claude-opus-4-7"},
		// Vertex @-date syntax becomes hyphenated.
		{"claude-3-7-sonnet@20250219", "claude-3-7-sonnet-20250219"},
		{"claude-sonnet-4-5@20250929", "claude-sonnet-4-5-20250929"},
		// Things we should NOT strip.
		{"gpt-4o-mini", "gpt-4o-mini"},
		{"o3", "o3"},
		{"models/gemini-2.0-flash-001", "models/gemini-2.0-flash-001"},
	}
	for _, c := range cases {
		got := normalizeModelID(c.in)
		assert.Equal(t, c.want, got, "normalizeModelID(%q)", c.in)
	}
}

// Test_GetSonnet46_BedrockKeyed confirms a plain "claude-sonnet-4-6"
// request resolves through the normalized fallback when the JSON only
// has the Bedrock-prefixed key for it.
func Test_GetSonnet46_BedrockKeyed(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	require.NoError(t, err)

	mi, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-6",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, mi.Pricing.Completion, "sonnet 4-6 must resolve to a priced entry")
}

// Test_GetOpus47_NewModel confirms claude-opus-4-7 (which only appears
// under both plain and Vertex-regional keys in fresh dumps) resolves.
func Test_GetOpus47_NewModel(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	require.NoError(t, err)

	mi, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "anthropic",
		Model:    "claude-opus-4-7",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, mi.Pricing.Completion)
}

func Test_GetAnthropicSubscriptionAliases(t *testing.T) {
	b, err := NewBaseModelInfoProvider()
	require.NoError(t, err)

	for _, alias := range []string{"opus", "opus[1m]", "claude-subscription"} {
		t.Run(alias, func(t *testing.T) {
			mi, err := b.GetModelInfo(context.Background(), &ModelInfoRequest{
				Provider: "anthropic",
				Model:    alias,
			})
			require.NoError(t, err)
			assert.Equal(t, "anthropic", mi.ProviderSlug)
			assert.Contains(t, mi.ProviderModelID, "claude-opus-")
			assert.NotEmpty(t, mi.Pricing.Prompt)
		})
	}
}

// TestGPT56Family_SupportsNoneReasoningEffort pins the catalog data the
// reasoning-effort gate depends on. The GPT-5.6 models default to "medium"
// reasoning effort, and OpenAI rejects that combination with function tools
// ("Function tools with reasoning_effort are not supported for gpt-5.6-luna
// in /v1/chat/completions ... set reasoning_effort to 'none'"), so Helix has
// to send an explicit "none" — which it only does when the model advertises
// support for it.
func TestGPT56Family_SupportsNoneReasoningEffort(t *testing.T) {
	provider, err := NewBaseModelInfoProvider()
	require.NoError(t, err)

	for _, modelID := range []string{"gpt-5.6-luna", "gpt-5.6-sol", "gpt-5.6-terra"} {
		t.Run(modelID, func(t *testing.T) {
			info, err := provider.GetModelInfo(context.Background(), &ModelInfoRequest{
				Provider: "openai",
				Model:    modelID,
			})
			require.NoError(t, err)
			assert.Contains(t, info.SupportedReasoningEfforts, "none")
			assert.True(t, info.SupportsReasoningEffort)
		})
	}
}

// TestOSeriesDoesNotSupportNoneReasoningEffort is the other half of the gate:
// o1/o3 predate the "none" value, so "none" must keep being stripped for them
// rather than sent through.
func TestOSeriesDoesNotSupportNoneReasoningEffort(t *testing.T) {
	provider, err := NewBaseModelInfoProvider()
	require.NoError(t, err)

	info, err := provider.GetModelInfo(context.Background(), &ModelInfoRequest{
		Provider: "openai",
		Model:    "o3-mini",
	})
	require.NoError(t, err)
	assert.NotContains(t, info.SupportedReasoningEfforts, "none")
}
