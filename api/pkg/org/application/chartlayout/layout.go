// Package chartlayout owns free-placed canvas coordinates for the
// helix-org chart UI. Pure presentation state: no org-graph invariants.
// The chart falls back to auto-layout (dagre) when a node has no saved
// position.
package chartlayout

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

// Service is the application-layer home for chart canvas positions.
type Service struct {
	positions store.ChartPositions
	now       func() time.Time
}

// Deps are the constructor-injected collaborators.
type Deps struct {
	Positions store.ChartPositions
	Now       func() time.Time
}

// New constructs a chartlayout Service. Positions may be nil — methods
// then return a clear error so test wirings that skip layout still
// compile.
func New(deps Deps) *Service {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	return &Service{positions: deps.Positions, now: now}
}

// List returns every saved position for the org.
func (s *Service) List(ctx context.Context, orgID string) ([]orgchart.ChartPosition, error) {
	if s.positions == nil {
		return nil, fmt.Errorf("chart positions store not wired")
	}
	if orgID == "" {
		return nil, fmt.Errorf("orgID is empty")
	}
	return s.positions.List(ctx, orgID)
}

// PositionInput is one node coordinate the UI wants to persist.
type PositionInput struct {
	Kind string
	ID   string
	X    float64
	Y    float64
}

// Upsert validates and persists one or more positions. Empty input is
// a no-op.
func (s *Service) Upsert(ctx context.Context, orgID string, inputs []PositionInput) ([]orgchart.ChartPosition, error) {
	if s.positions == nil {
		return nil, fmt.Errorf("chart positions store not wired")
	}
	if orgID == "" {
		return nil, fmt.Errorf("orgID is empty")
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	now := s.now().UTC()
	out := make([]orgchart.ChartPosition, 0, len(inputs))
	for _, in := range inputs {
		pos, err := orgchart.NewChartPosition(orgID, in.Kind, in.ID, in.X, in.Y, now)
		if err != nil {
			return nil, err
		}
		out = append(out, pos)
	}
	if err := s.positions.UpsertMany(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Clear drops every saved position for the org (reset to auto-layout).
func (s *Service) Clear(ctx context.Context, orgID string) error {
	if s.positions == nil {
		return fmt.Errorf("chart positions store not wired")
	}
	if orgID == "" {
		return fmt.Errorf("orgID is empty")
	}
	return s.positions.Clear(ctx, orgID)
}
