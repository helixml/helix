package scheduler

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
)

var (
	ErrRunnersAreFull     = errors.New("runner is full and no slots are stale")
	ErrNoRunnersAvailable = errors.New("no runners available")
	ErrModelWontFit       = errors.New("model won't fit in any runner")
	ErrPendingSlotsFull   = errors.New("pending slots are full")
)

// ErrorHandlingStrategy is a function that handles errors returned by the scheduler.
// If the error is temporary and can be retried later, retry will return true. Otherwise it will
// return false and an error.
func ErrorHandlingStrategy(schedulerError error, work *Workload) (bool, error) {
	l := log.With().
		Str("request_id", work.ID()).
		Str("model_id", work.model.ID).
		Uint64("model_size", work.model.Memory).
		Logger()

	// If the runners are just full with work, keep the work in the queue and retry later.
	if errors.Is(schedulerError, ErrRunnersAreFull) {
		l.Trace().Err(schedulerError).Msgf("unable to schedule, retrying...")
		return true, nil
	}

	// If the pending slots are full, retry the request later.
	if errors.Is(schedulerError, ErrPendingSlotsFull) {
		l.Trace().Err(schedulerError).Msgf("unable to schedule, retrying...")
		return true, nil
	}

	// If there are no runners available, fail the request.
	if errors.Is(schedulerError, ErrNoRunnersAvailable) {
		l.Warn().Err(schedulerError).Msgf("no runners available to schedule work")
		return false, fmt.Errorf("no runners available to schedule work: %w", schedulerError)
	}

	// If the model won't fit in any available runner, fail the request.
	if errors.Is(schedulerError, ErrModelWontFit) {
		l.Warn().Err(schedulerError).Msgf("model won't fit in any runner, please add a bigger runner")
		return false, fmt.Errorf("model won't fit in any runner: %w", schedulerError)
	}

	// Else a generic error occurred, fail the request.
	return false, fmt.Errorf("scheduling session (%s): %w", work.ID(), schedulerError)
}
