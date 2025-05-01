package scheduler

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

func (s *Scheduler) filterRunnersByMemory(work *Workload, runnerIDs []string) ([]string, error) {
	if len(runnerIDs) == 0 {
		return nil, fmt.Errorf("no runners available")
	}

	var filteredRunners []string

	runnerMemory := make(map[string]uint64)
	for _, runnerID := range runnerIDs {
		runnerMemory[runnerID] = s.controller.TotalMemory(runnerID)
	}

	numRunnersWithNotEnoughTotalMemory := 0
	largestRunnerMemory := uint64(0)
	requiredMemory := work.model.Memory

	for runnerID, memory := range runnerMemory {
		if memory >= requiredMemory {
			filteredRunners = append(filteredRunners, runnerID)
		} else {
			numRunnersWithNotEnoughTotalMemory++
		}
		if memory > largestRunnerMemory {
			largestRunnerMemory = memory
		}
	}
	withWorkContext(&log.Logger, work).Debug().Interface("filtered_runners", filteredRunners).Msg("filtered runners")

	if numRunnersWithNotEnoughTotalMemory == len(runnerIDs) {
		return nil, fmt.Errorf("no runner has enough GPU memory for this workload (desired: %d, largest: %d): %w", requiredMemory, largestRunnerMemory, ErrModelWontFit)
	}

	return filteredRunners, nil
}

func (s *Scheduler) filterRunnersByModel(work *Workload, runnerIDs []string) ([]string, error) {
	if len(runnerIDs) == 0 {
		return nil, fmt.Errorf("no runners available")
	}

	var filteredRunners []string

	for _, runnerID := range runnerIDs {
		status, err := s.controller.GetStatus(runnerID)
		if err != nil {
			return nil, fmt.Errorf("failed to get runner status: %w", err)
		}

		for _, modelStatus := range status.Models {
			if !modelStatus.DownloadInProgress && modelStatus.ModelID == work.model.ID {
				filteredRunners = append(filteredRunners, runnerID)
			}
		}
	}

	if len(filteredRunners) == 0 {
		return nil, fmt.Errorf("no runner has the model %s", work.model.ID)
	}

	return filteredRunners, nil
}
