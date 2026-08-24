package helix

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/stretchr/testify/require"
)

func TestSaveRestartRequiredContainer_RoundTrips(t *testing.T) {
	ctx := context.Background()
	st := memory.New()

	require.NoError(t, SaveRestartRequiredContainer(ctx, st, "org-rr", "b-rr", "3f9a1c2b4d5e"))

	state, err := LoadState(ctx, st, "org-rr", "b-rr")
	require.NoError(t, err)
	require.Equal(t, "3f9a1c2b4d5e", state.RestartRequiredContainer)
}

// A bot whose sandbox is stopped has no container id. Persisting the
// empty value is the "no banner" case and must overwrite a previous
// stamp rather than being skipped.
func TestSaveRestartRequiredContainer_EmptyOverwrites(t *testing.T) {
	ctx := context.Background()
	st := memory.New()

	require.NoError(t, SaveRestartRequiredContainer(ctx, st, "org-rr", "b-rr", "3f9a1c2b4d5e"))
	require.NoError(t, SaveRestartRequiredContainer(ctx, st, "org-rr", "b-rr", ""))

	state, err := LoadState(ctx, st, "org-rr", "b-rr")
	require.NoError(t, err)
	require.Empty(t, state.RestartRequiredContainer)
}

func TestLoadState_NoStampIsEmpty(t *testing.T) {
	ctx := context.Background()
	st := memory.New()

	state, err := LoadState(ctx, st, "org-rr", "b-never-stamped")
	require.NoError(t, err)
	require.Empty(t, state.RestartRequiredContainer)
}
