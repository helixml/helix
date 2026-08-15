package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/types"
)

// TestQwen38RejectsHighAcceptsXHigh pins the finding that motivated this table.
// The provider serving qwen3.8-27b rejects "high" — the value the UI used to
// offer by default — while accepting "xhigh". Getting this backwards 400s every
// request of the turn and aborts it with no work done.
func TestQwen38RejectsHighAcceptsXHigh(t *testing.T) {
	profile, ok := LookupReasoningEfforts("qwen3.8-27b")
	require.True(t, ok, "qwen3.8-27b must be in the curated table")

	assert.Contains(t, profile.Supported, "xhigh")
	assert.Contains(t, profile.Supported, "medium")
	assert.Contains(t, profile.Supported, "low")
	assert.NotContains(t, profile.Supported, "high")
	assert.Contains(t, profile.Rejected, "high")
	assert.Equal(t, "xhigh", profile.Default)
	assert.Equal(t, types.EffortSourceProbed, profile.Source)
}

// TestClaudeUsesOutputConfigEffort guards the parameter name. Claude reads
// output_config.effort; a value sent as a top-level reasoning_effort is ignored
// with no error, so the failure looks like "the setting does nothing".
func TestClaudeUsesOutputConfigEffort(t *testing.T) {
	for _, modelID := range []string{"claude-opus-5", "claude-sonnet-5", "claude-opus-4-8"} {
		t.Run(modelID, func(t *testing.T) {
			profile, ok := LookupReasoningEfforts(modelID)
			require.True(t, ok)
			assert.Equal(t, types.EffortParamOutputConfigEffort, profile.Parameter)
			assert.Contains(t, profile.Supported, "xhigh")
			assert.Equal(t, "high", profile.Default)
		})
	}
}

// TestClaude46GenerationRejectsXHigh pins the generational split: xhigh arrived
// with Opus 4.7, so offering it on a 4.6-generation model is an error.
func TestClaude46GenerationRejectsXHigh(t *testing.T) {
	for _, modelID := range []string{"claude-opus-4-6", "claude-sonnet-4-6"} {
		t.Run(modelID, func(t *testing.T) {
			profile, ok := LookupReasoningEfforts(modelID)
			require.True(t, ok)
			assert.NotContains(t, profile.Supported, "xhigh")
			assert.Contains(t, profile.Rejected, "xhigh")
			assert.Contains(t, profile.Supported, "max")
		})
	}
}

// TestModelsWithoutEffortSupport covers the models that take no effort value at
// all — the UI must render no selector rather than a default one.
func TestModelsWithoutEffortSupport(t *testing.T) {
	for _, modelID := range []string{"claude-sonnet-4-5", "claude-haiku-4-5"} {
		t.Run(modelID, func(t *testing.T) {
			profile, ok := LookupReasoningEfforts(modelID)
			require.True(t, ok)
			assert.False(t, profile.SupportsEffort)
			assert.Empty(t, profile.Supported)
		})
	}
}

// TestLookupMatchesLongestFamilyPrefix covers dated and suffixed builds
// resolving to their family, which is how a deepseek-v4-flash-0731 deployment
// gets an answer without its own row.
func TestLookupMatchesLongestFamilyPrefix(t *testing.T) {
	tests := []struct {
		modelID    string
		wantFamily string
	}{
		{"deepseek-v4-flash-0731", "deepseek-v4-flash"},
		{"deepseek-v4-flash", "deepseek-v4-flash"},
		{"qwen3.8-27b-instruct", "qwen3.8-27b"},
		{"anthropic/claude-opus-5", "claude-opus-5"},
		{"ds4-flash-node06/qwen3.8-27b", "qwen3.8-27b"},
		{"claude-opus-4-8", "claude-opus-4-8"},
		{"CLAUDE-OPUS-5", "claude-opus-5"},
	}
	for _, tc := range tests {
		t.Run(tc.modelID, func(t *testing.T) {
			profile, ok := LookupReasoningEfforts(tc.modelID)
			require.True(t, ok)
			assert.Equal(t, tc.wantFamily, profile.Family)
		})
	}
}

// TestLookupUnknownModelReportsUnknown is the important negative case: an
// unknown model must yield no profile, so callers render no effort options
// rather than a guessed list the provider would reject.
func TestLookupUnknownModelReportsUnknown(t *testing.T) {
	for _, modelID := range []string{"", "   ", "llama-4-70b", "some-unknown-model"} {
		profile, ok := LookupReasoningEfforts(modelID)
		assert.False(t, ok, "model %q must not resolve", modelID)
		assert.Nil(t, profile)
	}
}

// TestProfileTableIsWellFormed catches data-entry mistakes in the table itself:
// a value cannot be both supported and rejected, and a default must be
// supported.
func TestProfileTableIsWellFormed(t *testing.T) {
	for _, profile := range ListReasoningEffortProfiles() {
		t.Run(profile.Family, func(t *testing.T) {
			assert.NotEmpty(t, profile.Parameter, "parameter must be set")
			assert.NotEmpty(t, profile.Source, "source must be set")
			assert.NotEmpty(t, profile.VerifiedAt, "verified_at must be set")

			for _, rejected := range profile.Rejected {
				assert.NotContains(t, profile.Supported, rejected,
					"%q cannot be both supported and rejected", rejected)
			}
			if profile.SupportsEffort {
				assert.NotEmpty(t, profile.Supported)
				if profile.Default != "" {
					assert.Contains(t, profile.Supported, profile.Default,
						"default %q must be a supported value", profile.Default)
				}
			} else {
				assert.Empty(t, profile.Supported)
			}
		})
	}
}

// TestOverlayFillsCatalogueGapsOnly verifies the precedence rule: the bundled
// catalogue wins where it has data, and the curated table only fills gaps.
func TestOverlayFillsCatalogueGapsOnly(t *testing.T) {
	t.Run("fills a gap", func(t *testing.T) {
		info := &types.ModelInfo{}
		applyReasoningEffortProfile(info, "claude-opus-5")
		assert.True(t, info.SupportsReasoningEffort)
		assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, info.SupportedReasoningEfforts)
		assert.Equal(t, "high", info.DefaultReasoningEffort)
	})

	t.Run("does not override the catalogue", func(t *testing.T) {
		info := &types.ModelInfo{
			SupportedReasoningEfforts: []string{"catalogue-value"},
			DefaultReasoningEffort:    "catalogue-value",
		}
		applyReasoningEffortProfile(info, "claude-opus-5")
		assert.Equal(t, []string{"catalogue-value"}, info.SupportedReasoningEfforts)
		assert.Equal(t, "catalogue-value", info.DefaultReasoningEffort)
	})

	t.Run("leaves unknown models untouched", func(t *testing.T) {
		info := &types.ModelInfo{}
		applyReasoningEffortProfile(info, "some-unknown-model")
		assert.False(t, info.SupportsReasoningEffort)
		assert.Empty(t, info.SupportedReasoningEfforts)
	})

	t.Run("leaves effort-less models untouched", func(t *testing.T) {
		info := &types.ModelInfo{}
		applyReasoningEffortProfile(info, "claude-haiku-4-5")
		assert.False(t, info.SupportsReasoningEffort)
		assert.Empty(t, info.SupportedReasoningEfforts)
	})
}

// TestGetModelInfoAppliesOverlay checks the wiring end-to-end through the real
// provider, including that a model absent from the catalogue still errors (the
// billing gate depends on that error).
func TestGetModelInfoAppliesOverlay(t *testing.T) {
	provider, err := NewBaseModelInfoProvider()
	require.NoError(t, err)

	t.Run("unknown model still errors", func(t *testing.T) {
		_, err := provider.GetModelInfo(context.Background(), &ModelInfoRequest{
			Provider: "ds4-flash-node06",
			Model:    "qwen3.8-27b",
		})
		require.Error(t, err, "a model with no catalogue entry must not become priceable")
	})

	t.Run("catalogue model keeps its own efforts", func(t *testing.T) {
		info, err := provider.GetModelInfo(context.Background(), &ModelInfoRequest{
			Provider: "deepseek",
			Model:    "deepseek-v4-flash",
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"high", "xhigh"}, info.SupportedReasoningEfforts)
		assert.Equal(t, "high", info.DefaultReasoningEffort)
	})
}
