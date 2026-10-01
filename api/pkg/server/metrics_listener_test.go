package server

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

func TestDesktopHostAvailable(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name  string
		hosts []*types.SandboxInstance
		want  float64
	}{
		{
			name: "available",
			hosts: []*types.SandboxInstance{{
				Status:     sandboxInstanceStatusOnline,
				LastSeen:   now,
				GPUVendor:  "nvidia",
				RenderNode: "/dev/dri/renderD128",
			}},
			want: 1,
		},
		{
			name: "stale",
			hosts: []*types.SandboxInstance{{
				Status:     sandboxInstanceStatusOnline,
				LastSeen:   now.Add(-config.DefaultSandboxDispatchStaleThreshold),
				GPUVendor:  "nvidia",
				RenderNode: "/dev/dri/renderD128",
			}},
			want: 0,
		},
		{
			name: "not desktop capable",
			hosts: []*types.SandboxInstance{{
				Status:     sandboxInstanceStatusOnline,
				LastSeen:   now,
				GPUVendor:  "none",
				RenderNode: "SOFTWARE",
			}},
			want: 0,
		},
		{
			name: "offline",
			hosts: []*types.SandboxInstance{{
				Status:     sandboxInstanceStatusOffline,
				LastSeen:   now,
				GPUVendor:  "nvidia",
				RenderNode: "/dev/dri/renderD128",
			}},
			want: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := desktopHostAvailable(test.hosts, now); got != test.want {
				t.Fatalf("desktopHostAvailable() = %v, want %v", got, test.want)
			}
		})
	}
}
