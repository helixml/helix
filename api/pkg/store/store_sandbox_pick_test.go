package store

import (
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func pickTestInstance(t *testing.T, id, gpuVendor string, active, maxSandboxes int, versions map[string]string) *types.SandboxInstance {
	t.Helper()
	blob, err := json.Marshal(versions)
	if err != nil {
		t.Fatalf("marshal versions: %v", err)
	}
	return &types.SandboxInstance{
		ID:              id,
		GPUVendor:       gpuVendor,
		ActiveSandboxes: active,
		MaxSandboxes:    maxSandboxes,
		DesktopVersions: blob,
	}
}

func TestPickSandboxInstance(t *testing.T) {
	ubuntu := map[string]string{"ubuntu": "abc123"}

	tests := []struct {
		name            string
		instances       []*types.SandboxInstance
		desktopType     string
		requiresDisplay bool
		wantID          string // "" means nil
	}{
		{
			name:   "no instances",
			wantID: "",
		},
		{
			name: "desktop picks first load-ordered render-capable host",
			instances: []*types.SandboxInstance{
				pickTestInstance(t, "a", "nvidia", 1, 20, ubuntu),
				pickTestInstance(t, "b", "nvidia", 2, 20, ubuntu),
			},
			desktopType:     "ubuntu",
			requiresDisplay: true,
			wantID:          "a",
		},
		{
			name: "desktop skips cpu-only host",
			instances: []*types.SandboxInstance{
				pickTestInstance(t, "nas", "none", 0, 20, ubuntu),
				pickTestInstance(t, "gpu", "nvidia", 5, 20, ubuntu),
			},
			desktopType:     "ubuntu",
			requiresDisplay: true,
			wantID:          "gpu",
		},
		{
			name: "headless prefers cpu-only host over less loaded gpu host",
			instances: []*types.SandboxInstance{
				pickTestInstance(t, "gpu", "nvidia", 0, 20, ubuntu),
				pickTestInstance(t, "nas", "none", 5, 20, ubuntu),
			},
			desktopType:     "ubuntu",
			requiresDisplay: false,
			wantID:          "nas",
		},
		{
			name: "headless falls back to gpu host when no cpu-only host qualifies",
			instances: []*types.SandboxInstance{
				pickTestInstance(t, "nas", "none", 0, 20, map[string]string{"sway": "zzz"}),
				pickTestInstance(t, "gpu", "nvidia", 5, 20, ubuntu),
			},
			desktopType:     "ubuntu",
			requiresDisplay: false,
			wantID:          "gpu",
		},
		{
			name: "host at max_sandboxes ceiling is skipped",
			instances: []*types.SandboxInstance{
				pickTestInstance(t, "full", "nvidia", 20, 20, ubuntu),
				pickTestInstance(t, "free", "nvidia", 21, 40, ubuntu),
			},
			desktopType:     "ubuntu",
			requiresDisplay: true,
			wantID:          "free",
		},
		{
			name: "all hosts at ceiling yields nil",
			instances: []*types.SandboxInstance{
				pickTestInstance(t, "full", "nvidia", 20, 20, ubuntu),
			},
			desktopType:     "ubuntu",
			requiresDisplay: true,
			wantID:          "",
		},
		{
			name: "zero max_sandboxes means no ceiling",
			instances: []*types.SandboxInstance{
				pickTestInstance(t, "legacy", "nvidia", 50, 0, ubuntu),
			},
			desktopType:     "ubuntu",
			requiresDisplay: true,
			wantID:          "legacy",
		},
		{
			name: "host missing the requested image is skipped",
			instances: []*types.SandboxInstance{
				pickTestInstance(t, "sway-only", "nvidia", 0, 20, map[string]string{"sway": "zzz"}),
				pickTestInstance(t, "ubuntu-host", "nvidia", 5, 20, ubuntu),
			},
			desktopType:     "ubuntu",
			requiresDisplay: true,
			wantID:          "ubuntu-host",
		},
		{
			name: "host with empty version string is skipped",
			instances: []*types.SandboxInstance{
				pickTestInstance(t, "empty", "nvidia", 0, 20, map[string]string{"ubuntu": ""}),
			},
			desktopType:     "ubuntu",
			requiresDisplay: true,
			wantID:          "",
		},
		{
			name: "invalid version JSON is skipped",
			instances: []*types.SandboxInstance{
				{ID: "bad", GPUVendor: "nvidia", MaxSandboxes: 20, DesktopVersions: []byte("{not json")},
				pickTestInstance(t, "good", "nvidia", 5, 20, ubuntu),
			},
			desktopType:     "ubuntu",
			requiresDisplay: true,
			wantID:          "good",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pickSandboxInstance(tc.instances, tc.desktopType, tc.requiresDisplay)
			gotID := ""
			if got != nil {
				gotID = got.ID
			}
			if gotID != tc.wantID {
				t.Errorf("pickSandboxInstance() = %q, want %q", gotID, tc.wantID)
			}
		})
	}
}
