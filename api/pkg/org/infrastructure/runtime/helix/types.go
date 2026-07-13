package helix

import (
	"github.com/helixml/helix/api/pkg/types"
)

// IsTerminalOutput reports whether the session is in a terminal state.
// Free function because types.SessionOutputResponse lives in another
// package — we don't own the type and won't shim it with a method here.
func IsTerminalOutput(o types.SessionOutputResponse) bool {
	return o.Status == "complete" || o.Status == "error" || o.Status == "interrupted"
}

// ServerStatus mirrors the slice of /api/v1/config helix-org reads.
type ServerStatus struct {
	MaxConcurrentDesktops    int `json:"max_concurrent_desktops"`
	ActiveConcurrentDesktops int `json:"active_concurrent_desktops"`
}

// HasDesktopRoom reports whether at least one desktop slot is free.
// Max=0 means "unlimited" at the server level.
func (s ServerStatus) HasDesktopRoom() bool {
	if s.MaxConcurrentDesktops <= 0 {
		return true
	}
	return s.ActiveConcurrentDesktops < s.MaxConcurrentDesktops
}
