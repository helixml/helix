package scheduler

import (
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog/log"
)

type Cluster interface {
	UpdateRunner(props *types.RunnerState)
	DeadRunnerIDs() []string
	RunnerIDs() []string
	TotalMemory(runnerID string) uint64
}

type cluster struct {
	runners           *xsync.MapOf[string, *runner] // Maps a runner ID to its properties.
	runnerTimeoutFunc TimeoutFunc                   // Function to check if runners have timed out.
}

var _ Cluster = &cluster{}

// NOTE(milosgajdos): we really should make sure we return exported types.
// If we want the type fields to be inaccessible we should make them unexported.
// nolint:revive
func NewCluster(runnerTimeoutFunc TimeoutFunc) *cluster {
	return &cluster{
		runners:           xsync.NewMapOf[string, *runner](),
		runnerTimeoutFunc: runnerTimeoutFunc,
	}
}

// UpdateRunner updates the state of a runner and reconciles its slots with the allocator's records.
func (c *cluster) UpdateRunner(props *types.RunnerState) {
	log.Trace().
		Str("runner_id", props.ID).
		Int64("total_memory", int64(props.TotalMemory)).
		Int64("free_memory", props.FreeMemory).
		Interface("slots", props.Slots).
		Msg("updating runner state")

	// Update runner properties and activity.
	runner, _ := c.runners.LoadOrStore(props.ID, &runner{})
	runner.Update(props)
}

func (c *cluster) DeadRunnerIDs() []string {
	deadRunners := make([]string, 0)

	// Iterate through runners to check if any have timed out.
	c.runners.Range(func(runnerID string, r *runner) bool {
		if r.HasTimedOut(c.runnerTimeoutFunc) {
			deadRunners = append(deadRunners, runnerID)
		}
		return true
	})

	for _, runnerID := range deadRunners {
		log.Warn().
			Str("runner_id", runnerID).
			Msg("runner timed out")
		c.runners.Delete(runnerID)
	}

	return deadRunners
}

func (c *cluster) RunnerIDs() []string {
	return Keys(c.runners)
}

func (c *cluster) TotalMemory(runnerID string) uint64 {
	runner, ok := c.runners.Load(runnerID)
	if !ok {
		return 0
	}
	return runner.TotalMemory()
}

type runner struct {
	RunnerProperties   *types.RunnerState
	RunnerLastActivity time.Time
}

func (r *runner) Update(s *types.RunnerState) {
	r.RunnerProperties = s
	r.RunnerLastActivity = time.Now()
}

func (r *runner) HasTimedOut(timeoutFunc TimeoutFunc) bool {
	return timeoutFunc(r.RunnerProperties.ID, r.RunnerLastActivity)
}

func (r *runner) TotalMemory() uint64 {
	return r.RunnerProperties.TotalMemory
}
