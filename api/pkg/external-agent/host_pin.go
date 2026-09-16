package external_agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
)

// DesktopTypeRequiresDisplay reports whether a desktop type needs a
// display-capable sandbox host. Only "headless" runs without a compositor
// and encoder; every other type is a streamed desktop.
func DesktopTypeRequiresDisplay(desktopType string) bool {
	return !strings.EqualFold(desktopType, "headless")
}

// ValidateSandboxHostPin checks that an explicitly requested sandbox host
// exists, is online, and satisfies the workload's display requirement.
// Pinned placement bypasses the dispatcher's eligibility filters, so this is
// the only gate between a user's host choice and a container that would FATAL
// on a display-incapable host or dial a dead RevDial key. Capacity is
// deliberately not checked — it is transient, and a user pinning a host is
// asking for that host, full or not.
//
// An empty hostID means "dispatcher chooses" and always validates.
func ValidateSandboxHostPin(ctx context.Context, s store.Store, hostID string, requiresDisplay bool) error {
	if hostID == "" {
		return nil
	}
	host, err := s.GetSandboxInstance(ctx, hostID)
	if err != nil {
		return fmt.Errorf("sandbox host %q not found: %w", hostID, err)
	}
	if host.Status != "online" {
		return fmt.Errorf("sandbox host %q is %s, not online", hostID, host.Status)
	}
	if requiresDisplay && !host.CanHostDesktop() {
		return fmt.Errorf("sandbox host %q cannot run a streamed desktop (gpu_vendor=%q, render_node=%q); pick a display-capable host or use the headless runtime", hostID, host.GPUVendor, host.RenderNode)
	}
	return nil
}
