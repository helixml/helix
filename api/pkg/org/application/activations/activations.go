// Package activations owns the host-driven activation use case behind
// the REST "activate worker" endpoint: pre-allocating the audit row so
// the handler can return an activation id synchronously while the
// Spawner picks the row up (matched by Trigger.ActivationID) and
// completes it. This keeps the api adapter free of a direct
// activation.Repository reference.
package activations

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// Activations owns activation-row creation. Narrow by design: just the
// repository, a clock, and an id-generator.
type Activations struct {
	repo  activation.Repository
	now   func() time.Time
	newID func() string
}

// Deps are the constructor-injected collaborators for New. Repo may be
// nil (the activate endpoint then skips the audit pre-create, as the old
// inline code did when Activations was unwired).
type Deps struct {
	Repo  activation.Repository
	Now   func() time.Time
	NewID func() string
}

// New constructs the Activations service.
func New(deps Deps) *Activations {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Activations{repo: deps.Repo, now: now, newID: deps.NewID}
}

// PrepareManual pre-allocates a TriggerManual activation audit row for
// the Worker and returns its id. Returns an empty id (and nil error)
// when the repository or id-generator is unwired — callers treat that as
// "no pre-allocation; the Spawner mints its own", matching the previous
// inline behaviour.
func (a *Activations) PrepareManual(ctx context.Context, orgID string, workerID orgchart.WorkerID) (activation.ID, error) {
	if a.repo == nil || a.newID == nil {
		return "", nil
	}
	id := activation.ID("a-" + a.newID())
	act, err := activation.New(
		id,
		workerID,
		[]activation.Trigger{{Kind: activation.TriggerManual}},
		a.now(),
		orgID,
	)
	if err != nil {
		return "", fmt.Errorf("build manual activation: %w", err)
	}
	if err := a.repo.Create(ctx, act); err != nil {
		return "", fmt.Errorf("persist manual activation: %w", err)
	}
	return id, nil
}
