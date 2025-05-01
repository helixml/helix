package scheduler

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func Test_filterRunnersByMemory_NoRunners(t *testing.T) {
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	ctrl, err := NewRunnerController(context.Background(), &RunnerControllerConfig{
		PubSub: ps,
	})
	require.NoError(t, err)

	scheduler, err := NewScheduler(context.Background(), &config.ServerConfig{}, &Params{
		RunnerController: ctrl,
	})
	require.NoError(t, err)

	runners, err := scheduler.filterRunnersByMemory(&Workload{
		model: &types.Model{
			Memory: 1000,
		},
	}, []string{})
	require.Error(t, err)
	require.Nil(t, runners)
}
