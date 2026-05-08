package profile

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/helixml/helix/api/pkg/runner/gpuarch"
	"github.com/helixml/helix/api/pkg/types"
)

// RunnerGPUInfo is the per-GPU information a runner reports that the
// profile-compatibility check needs. Defined here as a distinct struct (not
// just types.GPUStatus) so the check is testable in isolation and so the
// API server can populate it from whichever runner status fields it has.
type RunnerGPUInfo struct {
	Index       int    // GPU index on the runner (0, 1, 2, ...)
	Vendor      types.GPUVendor // "nvidia" or "amd"
	Architecture string // canonical arch from gpuarch (e.g. "hopper")
	ModelName   string // marketing name (e.g. "NVIDIA H100 80GB HBM3")
	TotalVRAM   uint64 // bytes
}

// IncompatibilityReason is returned by Compatibility() when a profile's
// requirements aren't met by a runner's GPUs. The Constraint field names
// which one of the five checks failed; Detail is human-readable.
type IncompatibilityReason struct {
	Constraint string // one of: "count", "index", "vendor", "architecture", "model_match", "min_vram"
	Detail     string
}

func (r *IncompatibilityReason) Error() string {
	return fmt.Sprintf("incompatible: %s — %s", r.Constraint, r.Detail)
}

// Compatibility checks whether a runner with the given GPUs can host the
// given profile. Returns nil if compatible, or *IncompatibilityReason
// naming the failing constraint.
//
// Checks run in this order (cheapest first, fail fast):
//  1. count            — runner has at least profile.Count GPUs
//  2. index            — every GPU index referenced in the profile exists on the runner (skipped if not yet plumbed)
//  3. vendor           — every GPU on the runner matches profile.Vendor (if set)
//  4. architecture     — every GPU's architecture is in profile.Architectures (if non-empty)
//  5. model_match      — every GPU's marketing name matches profile.ModelMatch regex (if set)
//  6. min_vram         — every GPU's TotalVRAM >= profile.MinVRAMBytes (if set)
//
// A profile with all-empty optional constraints (only Count set) just needs
// enough GPUs of any kind.
func Compatibility(req types.ProfileGPURequirement, runnerGPUs []RunnerGPUInfo) error {
	// 1. count
	if req.Count > len(runnerGPUs) {
		return &IncompatibilityReason{
			Constraint: "count",
			Detail:     fmt.Sprintf("profile requires %d GPUs, runner has %d", req.Count, len(runnerGPUs)),
		}
	}
	// (Index existence is checked at the assignment layer where we know
	// which compose service references which device_ids — not here. The
	// pure compatibility check operates on a profile's declared count and
	// the runner's GPU inventory.)

	// 3. vendor
	if req.Vendor != "" {
		for _, g := range runnerGPUs {
			if g.Vendor != req.Vendor {
				return &IncompatibilityReason{
					Constraint: "vendor",
					Detail: fmt.Sprintf("profile requires vendor %q, runner GPU %d is vendor %q",
						req.Vendor, g.Index, g.Vendor),
				}
			}
		}
	}
	// 4. architecture
	if len(req.Architectures) > 0 {
		allowed := map[string]struct{}{}
		for _, a := range req.Architectures {
			allowed[a] = struct{}{}
		}
		for _, g := range runnerGPUs {
			if _, ok := allowed[g.Architecture]; !ok {
				return &IncompatibilityReason{
					Constraint: "architecture",
					Detail: fmt.Sprintf("profile requires one of %v, runner GPU %d is %q",
						req.Architectures, g.Index, g.Architecture),
				}
			}
		}
	}
	// 5. model_match
	if req.ModelMatch != "" {
		re, err := regexp.Compile(req.ModelMatch)
		if err != nil {
			// Bad regex is a profile authoring error, not a runner mismatch.
			return &IncompatibilityReason{
				Constraint: "model_match",
				Detail:     fmt.Sprintf("profile model_match regex %q is invalid: %v", req.ModelMatch, err),
			}
		}
		for _, g := range runnerGPUs {
			if !re.MatchString(g.ModelName) {
				return &IncompatibilityReason{
					Constraint: "model_match",
					Detail: fmt.Sprintf("profile requires GPU model matching %q, runner GPU %d is %q",
						req.ModelMatch, g.Index, g.ModelName),
				}
			}
		}
	}
	// 6. min_vram
	if req.MinVRAMBytes > 0 {
		for _, g := range runnerGPUs {
			if int64(g.TotalVRAM) < req.MinVRAMBytes {
				return &IncompatibilityReason{
					Constraint: "min_vram",
					Detail: fmt.Sprintf("profile requires >=%d bytes VRAM per GPU, runner GPU %d has %d",
						req.MinVRAMBytes, g.Index, g.TotalVRAM),
				}
			}
		}
	}
	return nil
}

// IsIncompatibility reports whether err is an *IncompatibilityReason. Used
// by HTTP handlers to distinguish "profile genuinely doesn't fit" (return
// 422 with detail) from infrastructure errors (return 500).
func IsIncompatibility(err error) bool {
	var r *IncompatibilityReason
	return errors.As(err, &r)
}

// FilterCompatible returns the subset of profiles whose GPU requirements
// are satisfied by the given runner GPUs. Used by the
// /api/v1/runners/{id}/compatible-profiles endpoint that populates the
// admin UI's profile-selection dropdown.
func FilterCompatible(profiles []*types.RunnerProfile, runnerGPUs []RunnerGPUInfo) []*types.RunnerProfile {
	out := make([]*types.RunnerProfile, 0, len(profiles))
	for _, p := range profiles {
		if Compatibility(p.GPURequirement, runnerGPUs) == nil {
			out = append(out, p)
		}
	}
	return out
}

// IndirectArchCheck is a convenience for callers that only have an NVIDIA
// compute capability or AMD gfx string. Maps to the canonical arch and
// returns whether it's in the allowed list. Empty allowed list = always ok.
func IndirectArchCheck(allowed []string, vendor types.GPUVendor, ccOrGFX string) bool {
	if len(allowed) == 0 {
		return true
	}
	var arch string
	switch vendor {
	case types.GPUVendorNVIDIA:
		arch = gpuarch.FromNVIDIAComputeCapability(ccOrGFX)
	case types.GPUVendorAMD:
		arch = gpuarch.FromAMDGFX(ccOrGFX)
	}
	if arch == "" {
		return false
	}
	for _, a := range allowed {
		if a == arch {
			return true
		}
	}
	return false
}
