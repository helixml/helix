package scheduler

import (
	"fmt"
	"math"
	"slices"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/rs/zerolog/log"
)

type SchedulingStrategy string

const (
	SchedulingStrategy_None           SchedulingStrategy = ""
	SchedulingStrategy_MaxUtilization SchedulingStrategy = "max_utilization"
	SchedulingStrategy_MaxSpread      SchedulingStrategy = "max_spread"
)

type SchedulingStrategyFunc func(Cluster, WorkloadAllocator, *Workload) (string, error)

func MaxUtilizationStrategy(c Cluster, a WorkloadAllocator, req *Workload) (string, error) {
	// Memory requirements for the model.
	modelRequirement := req.Model().GetMemoryRequirements(req.Mode())

	// Prioritize runners to minimize utilization.
	prioritizedRunners := runnersByMaxUtilisation(c, a)

	// Max available memory across all runners.
	maxMemory := maxMemory(c)

	// Find the first runner that can fit the workload.
	bestRunnerID, err := firstRunnerThatCanFit(c, a, modelRequirement, prioritizedRunners)
	if err != nil {
		return "", err
	}

	// Validate response or throw error
	return validateBestRunner(bestRunnerID, modelRequirement, maxMemory, prioritizedRunners)
}

// This strategy attempts to spread work across all runners, effectively a min utilization strategy
func MaxSpreadStrategy(c Cluster, a WorkloadAllocator, req *Workload) (string, error) {
	// Memory requirements for the model.
	modelRequirement := req.Model().GetMemoryRequirements(req.Mode())

	// Prioritize runners to minimize utilization.
	prioritizedRunners := Reverse(runnersByMaxUtilisation(c, a))

	// Max available memory across all runners.
	maxMemory := maxMemory(c)

	// Find the first runner that can fit the workload.
	bestRunnerID, err := firstRunnerThatCanFit(c, a, modelRequirement, prioritizedRunners)
	if err != nil {
		return "", err
	}

	// Validate response or throw error
	return validateBestRunner(bestRunnerID, modelRequirement, maxMemory, prioritizedRunners)
}

// DeleteMostStaleStrategy iteratively deletes allocated work from stale slots until there is enough
// memory to allocate the new workload.
func DeleteMostStaleStrategy(a WorkloadAllocator, runnerID string, runnerMem uint64, requiredMem uint64) error {
	for {
		allSlots := a.RunnerSlots(runnerID)
		staleSlots := Filter(allSlots, func(slot *Slot) bool {
			return slot.IsStale()
		})
		// If there is enough free space on the runner, break out of the loop.
		if requiredMem <= runnerMem-memoryUsed(allSlots) {
			break
		}
		// Sort the slots by last activity time
		slices.SortFunc(staleSlots, func(i, j *Slot) int {
			return int(i.lastActivityTime.Sub(j.lastActivityTime))
		})
		if len(staleSlots) == 0 {
			return fmt.Errorf("unable to find stale slot to replace")
		}
		// Then delete the most stale slot
		log.Debug().Str("slot_id", staleSlots[0].ID.String()).Msg("deleting stale slot")
		a.DeleteSlot(staleSlots[0].ID)
	}
	return nil
}

func memoryUsed(slots []*Slot) uint64 {
	var total uint64
	for _, slot := range slots {
		total += slot.Memory()
	}
	return total
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
			m, err := model.GetModel(string(slot.ModelName()))
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

// Validates and returns the best runner or an error
func validateBestRunner(bestRunnerID string, modelRequirement uint64, maxMemory uint64, prioritizedRunners []string) (string, error) {
	if bestRunnerID == "" {
		if len(prioritizedRunners) == 0 {
			return "", ErrNoRunnersAvailable
		}
		if maxMemory < modelRequirement {
			log.Trace().Uint64("max_memory", maxMemory).Float64("model_requirement", float64(modelRequirement)).Msg("model won't fit in any runner")
			return "", ErrModelWontFit
		}
		return "", ErrRunnersAreFull
	}
	return bestRunnerID, nil
}

// Find the first runner in the list that can fit the workload
func firstRunnerThatCanFit(c Cluster, a WorkloadAllocator, modelRequirement uint64, prioritizedRunners []string) (string, error) {
	var bestRunnerID string
	for _, runner := range prioritizedRunners {
		available, err := availableMemory(c, a, runner)
		if err != nil {
			return "", fmt.Errorf("getting available memory for runner (%s): %v", runner, err)
		}
		if available >= modelRequirement {
			bestRunnerID = runner
			break
		}
	}
	return bestRunnerID, nil
}

// maxMemory finds the largest amount of possible GPU memory across all runners
func maxMemory(c Cluster) uint64 {
	maxAvailable := float64(0)
	for _, runner := range c.RunnerIDs() {
		maxAvailable = math.Max(float64(c.TotalMemory(runner)), maxAvailable)
	}
	return uint64(maxAvailable)
}
