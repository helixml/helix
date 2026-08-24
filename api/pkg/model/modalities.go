package model

import (
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

// ModelModalityProfile records modality capabilities that providers do not
// expose through their OpenAI-compatible /v1/models endpoint.
type ModelModalityProfile struct {
	Family     string
	Input      []types.Modality
	Output     []types.Modality
	Source     string
	VerifiedAt string
}

var modelModalityProfiles = []ModelModalityProfile{
	{
		Family:     "qwen3.8-27b",
		Input:      []types.Modality{types.ModalityText, types.ModalityImage},
		Output:     []types.Modality{types.ModalityText},
		Source:     "probed",
		VerifiedAt: "2026-08-24",
	},
}

// LookupModelModalities returns the longest matching model-family profile.
// Unknown models deliberately return false instead of guessing attachment
// support, because OpenCode defaults custom models to text-only.
func LookupModelModalities(modelID string) (*ModelModalityProfile, bool) {
	normalized := strings.ToLower(strings.TrimSpace(modelID))
	if idx := strings.LastIndex(normalized, "/"); idx >= 0 {
		normalized = normalized[idx+1:]
	}
	if normalized == "" {
		return nil, false
	}

	best := -1
	for idx := range modelModalityProfiles {
		if !strings.HasPrefix(normalized, modelModalityProfiles[idx].Family) {
			continue
		}
		if best == -1 || len(modelModalityProfiles[idx].Family) > len(modelModalityProfiles[best].Family) {
			best = idx
		}
	}
	if best == -1 {
		return nil, false
	}

	profile := modelModalityProfiles[best]
	profile.Input = append([]types.Modality(nil), profile.Input...)
	profile.Output = append([]types.Modality(nil), profile.Output...)
	return &profile, true
}
