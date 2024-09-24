package scheduler

import (
	"fmt"
	"math"
	"slices"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/rs/zerolog/log"
)

type PlacementStrategy func(Cluster, WorkloadAllocator, *Workload) (string, error)

func MaxUtilizationStrategy(c Cluster, a WorkloadAllocator, req *Workload) (string, error) {
	// Prioritize runners to maximize utilization.
	prioritizedRunners := runnersByMaxUtilisation(c, a)

	// Find the first runner that can fit the workload.
	var bestRunnerID string
	maxAvailable := float64(0)
	modelRequirement := req.Model().GetMemoryRequirements(req.Mode())
	for _, runner := range prioritizedRunners {
		runnerTotal := c.TotalMemory(runner)
		maxAvailable = math.Max(float64(runnerTotal), maxAvailable)

		available, err := availableMemory(c, a, runner)
		if err != nil {
			return "", fmt.Errorf("getting available memory for runner (%s): %v", runner, err)
		}
		if available >= modelRequirement {
			bestRunnerID = runner
			break
		}
	}
	if bestRunnerID == "" {
		if len(prioritizedRunners) == 0 {
			return "", ErrNoRunnersAvailable
		}
		if maxAvailable < float64(modelRequirement) {
			log.Trace().Float64("max_available", maxAvailable).Float64("model_requirement", float64(modelRequirement)).Msg("model won't fit in any runner")
			return "", ErrModelWontFit
		}
		return "", ErrRunnersAreFull
	}

	return bestRunnerID, nil
}

// runnersByMaxUtilisation sorts runners by their available memory in descending order,
// prioritizing runners with the least free memory for better utilization.
func runnersByMaxUtilisation(c Cluster, a WorkloadAllocator) []string {
	runners := c.RunnerIDs()
	// Sort runners based on available memory, from least available to most available.
	slices.SortFunc(
		runners,
		func(i, j string) int {
			// Calculate available memory for runner i.
			memA, err := availableMemory(c, a, i)
			if err != nil {
				log.Err(err).Msg("sorting a by utilization")
			}
			// Calculate available memory for runner j.
			memB := uint64(0)
			if j != "" {
				memB, err = availableMemory(c, a, j)
				if err != nil {
					log.Err(err).Msg("sorting b by utilization")
				}
			}
			// Sort by available memory (descending).
			return int(memA) - int(memB)
		},
	)
	return runners
}

// availableMemory calculates the available memory for a specific runner by subtracting
// the memory usage of all non-stale slots assigned to that runner.
func availableMemory(c Cluster, a WorkloadAllocator, runnerID string) (uint64, error) {
	var available int64 // This could potentially be negative after subtracting too many things

	// Start with the total memory of the runner.
	available = int64(c.TotalMemory(runnerID))

	// Subtract the memory of all non-stale slots assigned to this runner.
	runnerSlots := a.RunnerSlots(runnerID)
	for _, slot := range runnerSlots {
		if !slot.IsStale() {
			// Get the memory requirements for the slot's model.
			m, err := model.GetModel(slot.ModelName())
			if err != nil {
				return 0, fmt.Errorf("getting slot model (%s): %v", slot.ModelName(), err)
			}
			available -= int64(m.GetMemoryRequirements(slot.Mode()))
		}
	}
	if available < 0 {
		available = 0
	}

	return uint64(available), nil
}
