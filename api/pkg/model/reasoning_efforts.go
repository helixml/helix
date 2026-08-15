package model

import (
	"sort"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

// This file is the curated record of which reasoning-effort values each model
// family actually accepts.
//
// It exists because the value is not discoverable at runtime. vLLM's /v1/models
// returns the plain OpenAI ModelCard (id, owned_by, max_model_len, permission)
// and says nothing about sampling parameters, so a self-hosted model's accepted
// effort set can only be learned by sending a request and reading the 400 — and
// some templates accept an unrecognized value silently instead of rejecting it,
// so probing for rejections alone does not give the supported set either.
//
// Passing an unsupported value is not a soft failure: the provider rejects the
// whole request, the coding agent retries the same 400 several times, and the
// turn aborts with no work done.
//
// Every profile carries the source it was established from and, where relevant,
// the values that are known to be REJECTED, so a mismatch between this table and
// a live provider can be triaged without re-deriving it. Adding a model is a
// data edit — append a profile, no code change.

// reasoningEffortProfiles is the curated table. Matching is longest-prefix over
// the normalized model id, so a dated build (qwen3.8-27b-20260731) resolves to
// its family entry without needing its own row.
var reasoningEffortProfiles = []types.ReasoningEffortProfile{
	// --- Anthropic -----------------------------------------------------------
	// Claude reads effort from output_config.effort. A top-level
	// reasoning_effort is not read, so a value sent there has no effect and
	// produces no error — the symptom is "the setting does nothing".
	// xhigh arrived with Opus 4.7; 4.6-generation models reject it.
	{
		Family: "claude-opus-5", Parameter: types.EffortParamOutputConfigEffort,
		Supported: []string{"low", "medium", "high", "xhigh", "max"},
		Default:   "high", SupportsEffort: true,
		Source: types.EffortSourceVendor, VerifiedAt: "2026-08-15",
		Notes: "Thinking is on by default. Disabling thinking is accepted only at effort high or below; pairing thinking:disabled with xhigh or max is a 400.",
	},
	{
		Family: "claude-fable-5", Parameter: types.EffortParamOutputConfigEffort,
		Supported: []string{"low", "medium", "high", "xhigh", "max"},
		Default:   "high", SupportsEffort: true,
		Source: types.EffortSourceVendor, VerifiedAt: "2026-08-15",
		Notes: "Thinking is always on; an explicit thinking:disabled is a 400 at any effort.",
	},
	{
		Family: "claude-mythos-5", Parameter: types.EffortParamOutputConfigEffort,
		Supported: []string{"low", "medium", "high", "xhigh", "max"},
		Default:   "high", SupportsEffort: true,
		Source: types.EffortSourceVendor, VerifiedAt: "2026-08-15",
		Notes: "Same API surface as claude-fable-5.",
	},
	{
		Family: "claude-opus-4-8", Parameter: types.EffortParamOutputConfigEffort,
		Supported: []string{"low", "medium", "high", "xhigh", "max"},
		Default:   "high", SupportsEffort: true,
		Source: types.EffortSourceVendor, VerifiedAt: "2026-08-15",
	},
	{
		Family: "claude-opus-4-7", Parameter: types.EffortParamOutputConfigEffort,
		Supported: []string{"low", "medium", "high", "xhigh", "max"},
		Default:   "high", SupportsEffort: true,
		Source: types.EffortSourceVendor, VerifiedAt: "2026-08-15",
		Notes: "First Claude model with xhigh.",
	},
	{
		Family: "claude-sonnet-5", Parameter: types.EffortParamOutputConfigEffort,
		Supported: []string{"low", "medium", "high", "xhigh", "max"},
		Default:   "high", SupportsEffort: true,
		Source: types.EffortSourceVendor, VerifiedAt: "2026-08-15",
		Notes: "First Sonnet-tier model with xhigh.",
	},
	{
		Family: "claude-opus-4-6", Parameter: types.EffortParamOutputConfigEffort,
		Supported: []string{"low", "medium", "high", "max"},
		Rejected:  []string{"xhigh"},
		Default:   "high", SupportsEffort: true,
		Source: types.EffortSourceVendor, VerifiedAt: "2026-08-15",
		Notes: "xhigh did not exist yet on the 4.6 generation.",
	},
	{
		Family: "claude-sonnet-4-6", Parameter: types.EffortParamOutputConfigEffort,
		Supported: []string{"low", "medium", "high", "max"},
		Rejected:  []string{"xhigh"},
		Default:   "high", SupportsEffort: true,
		Source: types.EffortSourceVendor, VerifiedAt: "2026-08-15",
	},
	{
		Family: "claude-opus-4-5", Parameter: types.EffortParamOutputConfigEffort,
		Supported: []string{"low", "medium", "high"},
		Rejected:  []string{"xhigh", "max"},
		Default:   "high", SupportsEffort: true,
		Source: types.EffortSourceVendor, VerifiedAt: "2026-08-15",
	},
	{
		Family: "claude-sonnet-4-5", Parameter: types.EffortParamOutputConfigEffort,
		SupportsEffort: false,
		Source:         types.EffortSourceVendor, VerifiedAt: "2026-08-15",
		Notes: "Predates the effort parameter; sending any effort value errors.",
	},
	{
		Family: "claude-haiku-4-5", Parameter: types.EffortParamOutputConfigEffort,
		SupportsEffort: false,
		Source:         types.EffortSourceVendor, VerifiedAt: "2026-08-15",
		Notes: "Predates the effort parameter; sending any effort value errors.",
	},

	// --- OpenAI --------------------------------------------------------------
	{
		Family: "gpt-5.5-pro", Parameter: types.EffortParamReasoningEffort,
		Supported: []string{"medium", "high", "xhigh"},
		Default:   "medium", SupportsEffort: true,
		Source: types.EffortSourceCatalogue, VerifiedAt: "2026-08-15",
		Notes: "Reasoning is mandatory on this model; there is no off setting.",
	},

	// --- DeepSeek ------------------------------------------------------------
	{
		Family: "deepseek-v4-flash", Parameter: types.EffortParamReasoningEffort,
		Supported: []string{"high", "xhigh"},
		Default:   "high", SupportsEffort: true,
		Source: types.EffortSourceCatalogue, VerifiedAt: "2026-08-15",
		Notes: "Catalogue entry describes the 20260423 build. Later dated builds (e.g. -0731) match this family by prefix and have NOT been probed separately; if one rejects high or xhigh, add a dated row above this one.",
	},
	{
		Family: "deepseek-v4-pro", Parameter: types.EffortParamReasoningEffort,
		Supported: []string{"high", "xhigh"},
		Default:   "high", SupportsEffort: true,
		Source: types.EffortSourceCatalogue, VerifiedAt: "2026-08-15",
	},

	// --- Qwen ----------------------------------------------------------------
	{
		Family: "qwen3.8-27b", Parameter: types.EffortParamReasoningEffort,
		Supported: []string{"none", "low", "medium", "xhigh"},
		Rejected:  []string{"high", "max", "minimal"},
		Default:   "xhigh", SupportsEffort: true,
		Source: types.EffortSourceProbed, VerifiedAt: "2026-08-15",
		Notes: "Probed against a vLLM 0.11.2 deployment. The chat template raises on the OpenAI ladder names high/max/minimal and SILENTLY COERCES any other unrecognized value to the xhigh default, so probing for rejections alone does not reveal the supported set. Note high is rejected while xhigh is accepted, which is the reverse of most models.",
	},
}

// normalizeModelIDForEffort lowercases and strips a provider prefix so
// "anthropic/claude-opus-5" and "claude-opus-5" resolve to the same profile.
func normalizeModelIDForEffort(modelID string) string {
	s := strings.ToLower(strings.TrimSpace(modelID))
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		s = s[idx+1:]
	}
	return s
}

// LookupReasoningEfforts returns the curated profile for a model id, matching on
// the longest family prefix so dated and quantized builds resolve to their
// family. It reports false when the model is not in the table — callers must not
// invent an effort list for an unknown model, because offering a value the
// provider rejects is what aborts a turn.
func LookupReasoningEfforts(modelID string) (*types.ReasoningEffortProfile, bool) {
	normalized := normalizeModelIDForEffort(modelID)
	if normalized == "" {
		return nil, false
	}

	best := -1
	for idx := range reasoningEffortProfiles {
		family := reasoningEffortProfiles[idx].Family
		if !strings.HasPrefix(normalized, family) {
			continue
		}
		if best == -1 || len(family) > len(reasoningEffortProfiles[best].Family) {
			best = idx
		}
	}
	if best == -1 {
		return nil, false
	}

	profile := reasoningEffortProfiles[best]
	return &profile, true
}

// ListReasoningEffortProfiles returns every curated profile, sorted by family,
// for the admin/debug surface.
func ListReasoningEffortProfiles() []types.ReasoningEffortProfile {
	out := make([]types.ReasoningEffortProfile, len(reasoningEffortProfiles))
	copy(out, reasoningEffortProfiles)
	sort.Slice(out, func(i, j int) bool { return out[i].Family < out[j].Family })
	return out
}

// applyReasoningEffortProfile fills in reasoning-effort fields the bundled
// catalogue does not carry. It only ever fills gaps: when model_info.json
// already lists efforts for a model, that entry wins, because it ships with the
// pricing data the rest of the catalogue is keyed on and is regenerated as a
// unit. The curated table is for models the catalogue does not describe.
func applyReasoningEffortProfile(info *types.ModelInfo, modelID string) {
	if info == nil || len(info.SupportedReasoningEfforts) > 0 {
		return
	}
	profile, ok := LookupReasoningEfforts(modelID)
	if !ok || !profile.SupportsEffort {
		return
	}
	info.SupportsReasoningEffort = true
	info.SupportedReasoningEfforts = profile.Supported
	info.DefaultReasoningEffort = profile.Default
}
