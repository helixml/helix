package services

import (
	"context"
	"fmt"
	"math"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// WolfScheduler handles Wolf instance selection for new sandbox allocation
type WolfScheduler struct {
	store store.Store
}

// NewWolfScheduler creates a new Wolf scheduler
func NewWolfScheduler(store store.Store) *WolfScheduler {
	return &WolfScheduler{
		store: store,
	}
}

// SelectWolfInstance picks the best available Wolf for a new sandbox
// Algorithm: Least-loaded Wolf with matching GPU type (if specified)
// Returns error if no Wolfs available or all at capacity
func (s *WolfScheduler) SelectWolfInstance(ctx context.Context, gpuType string) (*types.WolfInstance, error) {
	instances, err := s.store.ListWolfInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list Wolf instances: %w", err)
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("no Wolf instances available")
	}

	var bestWolf *types.WolfInstance
	lowestLoad := math.MaxFloat64

	for _, inst := range instances {
		// Filter by status - must be online
		if inst.Status != types.WolfInstanceStatusOnline {
			continue
		}

		// Filter by GPU type if specified
		if gpuType != "" && inst.GPUType != gpuType {
			continue
		}

		// Check if at capacity
		if inst.ConnectedSandboxes >= inst.MaxSandboxes {
			continue
		}

		// Calculate load ratio (lower is better)
		var load float64
		if inst.MaxSandboxes > 0 {
			load = float64(inst.ConnectedSandboxes) / float64(inst.MaxSandboxes)
		} else {
			load = 0
		}

		// Select Wolf with lowest load
		if load < lowestLoad {
			lowestLoad = load
			bestWolf = inst
		}
	}

	if bestWolf == nil {
		if gpuType != "" {
			return nil, fmt.Errorf("no available Wolf instances with GPU type '%s' (all offline or at capacity)", gpuType)
		}
		return nil, fmt.Errorf("no available Wolf instances (all offline or at capacity)")
	}

	return bestWolf, nil
}

// MarkWolfDegraded marks a Wolf as degraded if it fails health checks
func (s *WolfScheduler) MarkWolfDegraded(ctx context.Context, wolfID string) error {
	instance, err := s.store.GetWolfInstance(ctx, wolfID)
	if err != nil {
		return fmt.Errorf("failed to get Wolf instance: %w", err)
	}

	// Update status to degraded
	instance.Status = types.WolfInstanceStatusDegraded
	err = s.store.UpdateWolfStatus(ctx, wolfID, types.WolfInstanceStatusDegraded)
	if err != nil {
		return fmt.Errorf("failed to update Wolf status: %w", err)
	}

	return nil
}

// MarkWolfOffline marks a Wolf as offline (used by health monitor)
func (s *WolfScheduler) MarkWolfOffline(ctx context.Context, wolfID string) error {
	err := s.store.UpdateWolfStatus(ctx, wolfID, types.WolfInstanceStatusOffline)
	if err != nil {
		return fmt.Errorf("failed to mark Wolf offline: %w", err)
	}
	return nil
}
