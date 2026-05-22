package helix

import (
	"context"

	"github.com/helixml/helix/api/pkg/org/runtime"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/store"
)

// Hire is the helix-runtime's runtime.HireHook impl. It
// persists the hiring user's identifier on the Worker's runtime-state
// sidecar so the Spawner can later mint per-user identity for that
// Worker's sessions (see SpawnerConfig.BearerForUser).
//
// Replaces the direct agenthelix.SaveHiringUser call hire_worker used
// to make. Lifted to a port so non-helix runtimes (claude) can no-op
// without hire_worker knowing.
type Hire struct {
	Store *store.Store
}

// OnHire persists hiringUserID via SaveHiringUser. Empty userID is a
// no-op (preserves SaveHiringUser's existing contract).
func (h *Hire) OnHire(ctx context.Context, workerID worker.ID, hiringUserID string) error {
	return SaveHiringUser(ctx, h.Store, workerID, hiringUserID)
}

// Compile-time check that Hire satisfies the port.
var _ runtime.HireHook = (*Hire)(nil)
