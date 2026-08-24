package server

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	helixorgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

// fakeSessionReader satisfies the anonymous interface orgWorkerRuntime
// holds for the Helix session/app store.
type fakeSessionReader struct{ session *types.Session }

func (f fakeSessionReader) GetSession(_ context.Context, _ string) (*types.Session, error) {
	return f.session, nil
}
func (f fakeSessionReader) GetApp(_ context.Context, _ string) (*types.App, error) {
	return nil, nil
}

func runtimeFor(t *testing.T, stamp, containerID, agentStatus string) helixorgapi.BotRuntimeInfo {
	t.Helper()
	ctx := context.Background()
	st := memory.New()
	require.NoError(t, runtimehelix.SaveProject(ctx, st, "org-rr", "b-rr", "prj_1", "app_1", "repo_1"))
	require.NoError(t, runtimehelix.SaveSession(ctx, st, "org-rr", "b-rr", "ses_1"))
	require.NoError(t, runtimehelix.SaveRestartRequiredContainer(ctx, st, "org-rr", "b-rr", stamp))

	session := &types.Session{ID: "ses_1"}
	session.Metadata.ContainerID = containerID
	session.Metadata.ExternalAgentStatus = agentStatus

	info, err := orgWorkerRuntime{st: st, sessions: fakeSessionReader{session: session}}.
		State(ctx, "org-rr", "b-rr")
	require.NoError(t, err)
	return info
}

// The banner case: the same container that was live at save time is still
// running, so it is still serving the pre-edit tool list.
func TestState_RestartRequiredWhenStampMatchesLiveContainer(t *testing.T) {
	info := runtimeFor(t, "container-a", "container-a", "running")
	require.True(t, info.RestartRequired)
}

// The self-clearing property. Any recreate — stop/start, idle reap,
// crash reconcile, full restart — yields a new Docker id, so the stamp
// stops matching with no clearing code anywhere.
func TestState_RestartNotRequiredAfterContainerReplaced(t *testing.T) {
	info := runtimeFor(t, "container-a", "container-b", "running")
	require.False(t, info.RestartRequired)
}

func TestState_RestartNotRequiredWhenSandboxStopped(t *testing.T) {
	info := runtimeFor(t, "container-a", "", "stopped")
	require.False(t, info.RestartRequired)
}

// Editing config while the sandbox is down stamps "". That must never
// match, including against a session whose ContainerID is also "".
func TestState_EmptyStampNeverMatches(t *testing.T) {
	info := runtimeFor(t, "", "", "running")
	require.False(t, info.RestartRequired)
}

// fakeSessionGetter satisfies restartStampSessions with a configurable
// result, so stampRestartRequiredContainer's error paths are directly
// testable without a live session store.
type fakeSessionGetter struct {
	session *types.Session
	err     error
}

func (f fakeSessionGetter) GetSession(_ context.Context, _ string) (*types.Session, error) {
	return f.session, f.err
}

func TestStampRestartRequiredContainer_StampsLiveContainerID(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	require.NoError(t, runtimehelix.SaveSession(ctx, st, "org-stamp", "b-stamp", "ses_1"))

	session := &types.Session{ID: "ses_1"}
	session.Metadata.ContainerID = "container-x"

	stampRestartRequiredContainer(ctx, st, fakeSessionGetter{session: session}, "org-stamp", "b-stamp")

	ws, err := runtimehelix.LoadState(ctx, st, "org-stamp", "b-stamp")
	require.NoError(t, err)
	require.Equal(t, "container-x", ws.RestartRequiredContainer)
}

// The regression guard for the stamp-erasure bug: a transient session
// lookup error must NOT overwrite an already-showing banner with "".
func TestStampRestartRequiredContainer_LeavesPreviousStampOnSessionLookupError(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	require.NoError(t, runtimehelix.SaveSession(ctx, st, "org-stamp", "b-stamp", "ses_1"))
	require.NoError(t, runtimehelix.SaveRestartRequiredContainer(ctx, st, "org-stamp", "b-stamp", "previous-container"))

	stampRestartRequiredContainer(ctx, st, fakeSessionGetter{err: errors.New("transient lookup failure")}, "org-stamp", "b-stamp")

	ws, err := runtimehelix.LoadState(ctx, st, "org-stamp", "b-stamp")
	require.NoError(t, err)
	require.Equal(t, "previous-container", ws.RestartRequiredContainer)
}

func TestStampRestartRequiredContainer_StampsEmptyWhenNoSession(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	// No SaveSession call — SessionID stays "".

	stampRestartRequiredContainer(ctx, st, fakeSessionGetter{}, "org-stamp", "b-stamp")

	ws, err := runtimehelix.LoadState(ctx, st, "org-stamp", "b-stamp")
	require.NoError(t, err)
	require.Equal(t, "", ws.RestartRequiredContainer)
}

func TestStampRestartRequiredContainer_StampsEmptyWhenSandboxStopped(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	require.NoError(t, runtimehelix.SaveSession(ctx, st, "org-stamp", "b-stamp", "ses_1"))
	require.NoError(t, runtimehelix.SaveRestartRequiredContainer(ctx, st, "org-stamp", "b-stamp", "previous-container"))

	session := &types.Session{ID: "ses_1"} // ContainerID zero value ""

	stampRestartRequiredContainer(ctx, st, fakeSessionGetter{session: session}, "org-stamp", "b-stamp")

	ws, err := runtimehelix.LoadState(ctx, st, "org-stamp", "b-stamp")
	require.NoError(t, err)
	require.Equal(t, "", ws.RestartRequiredContainer)
}
