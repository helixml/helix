package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	runtimehelix "github.com/helixml/helix/api/pkg/org/runtime/helix"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
)

// newInProcTestSetup builds a NewTestServer-backed HelixAPIServer + an
// inProcHelixClient with a fixed service user. The single setup
// covers every test in this file — each test seeds whatever store
// state it needs against the returned memorystore.
//
// We deliberately leave `Cfg.Inference` / providers etc unset: the
// adapter's ProjectService methods we exercise here don't touch those
// fields (`applyProject` does, but it isn't covered in this test
// suite — that handler's full happy path is exercised separately
// via the in-Helix end-to-end tests).
func newInProcTestSetup(t *testing.T) (*HelixAPIServer, *memorystore.MemoryStore, *inProcHelixClient, *types.User) {
	t.Helper()
	store := memorystore.New()
	server := NewTestServer(store, pubsub.NewNoop())
	user := &types.User{
		ID:        "usr_service",
		Email:     "service@helix.local",
		FullName:  "Service User",
		Type:      types.OwnerTypeUser,
		TokenType: types.TokenTypeAPIKey,
	}
	client := NewInProcHelixClient(server, user)
	return server, store, client, user
}

// TestInProcProjectService_GetProject_NotFound_ReturnsErrProjectNotFound
// exercises the 404 → ErrProjectNotFound mapping that WorkerProject.Ensure's
// stale-pointer recovery path depends on (Ensure → GetProject errors.Is(…,
// ErrProjectNotFound) → ClearProject + re-apply).
func TestInProcProjectService_GetProject_NotFound_ReturnsErrProjectNotFound(t *testing.T) {
	_, _, client, _ := newInProcTestSetup(t)

	_, err := client.GetProject(context.Background(), "proj_missing")
	require.Error(t, err)
	require.True(t, errors.Is(err, runtimehelix.ErrProjectNotFound),
		"expected ErrProjectNotFound, got %v", err)
}

// TestInProcProjectService_GetProject_Found_ReturnsProject verifies the
// happy path — a seeded project row round-trips through the inProc
// adapter back to types.Project. Authorization passes because the
// adapter resolves the service user as the project owner.
func TestInProcProjectService_GetProject_Found_ReturnsProject(t *testing.T) {
	_, store, client, user := newInProcTestSetup(t)

	store.SeedProject(&types.Project{
		ID:     "proj_test_1",
		Name:   "test-project",
		UserID: user.ID, // owner == service user so authz passes
		Status: "active",
	})

	got, err := client.GetProject(context.Background(), "proj_test_1")
	require.NoError(t, err)
	require.Equal(t, "proj_test_1", got.ID)
	require.Equal(t, "test-project", got.Name)
}

// TestInProcSpawnerClient_GetOutput_ReturnsStoreData seeds a session
// + interactions and asserts GetOutput returns the data the handler
// would have served via /api/v1/sessions/{id}/output.
func TestInProcSpawnerClient_GetOutput_ReturnsStoreData(t *testing.T) {
	_, store, client, user := newInProcTestSetup(t)

	session, err := store.CreateSession(context.Background(), types.Session{
		ID:    "ses_test_1",
		Owner: user.ID, // owner == service user so authz passes (no orgID)
	})
	require.NoError(t, err)

	_, err = store.CreateInteraction(context.Background(), &types.Interaction{
		ID:              "int_test_1",
		SessionID:       session.ID,
		State:           types.InteractionStateComplete,
		ResponseMessage: "hello from the test",
	})
	require.NoError(t, err)

	out, err := client.GetOutput(context.Background(), "ses_test_1")
	require.NoError(t, err)
	require.Equal(t, "ses_test_1", out.SessionID)
	// Status comes from the last interaction's State (set above).
	require.Equal(t, string(types.InteractionStateComplete), out.Status)
}

// TestInProcProjectService_GetAppConfig_RoundTrips seeds an app row and
// asserts GetAppConfig returns its embedded AppConfig verbatim.
// Validates the JSON shape Helix's handler emits matches what the
// runtimehelix port expects (types.AppConfig directly, not raw JSON).
func TestInProcProjectService_GetAppConfig_RoundTrips(t *testing.T) {
	_, store, client, user := newInProcTestSetup(t)

	want := types.AppConfig{
		AllowedDomains: []string{"example.test"},
		Secrets:        map[string]string{"FOO": "bar"},
		Helix: types.AppHelixConfig{
			Name: "test-app",
		},
	}
	store.SeedApp(&types.App{
		ID:     "app_test_1",
		Owner:  user.ID, // owner == service user so authz passes (no orgID)
		Config: want,
	})

	got, err := client.GetAppConfig(context.Background(), "app_test_1")
	require.NoError(t, err)
	require.Equal(t, want.AllowedDomains, got.AllowedDomains)
	require.Equal(t, want.Secrets, got.Secrets)
	require.Equal(t, want.Helix.Name, got.Helix.Name)
}

// TestInProcSpawnerClient_StopExternalAgent_NoSession_ReturnsError
// confirms a missing session ID surfaces as an error (the underlying
// handler returns 404; the adapter wraps it as a generic error — no
// sentinel needed since SpawnerClient.StopExternalAgent has no
// sentinel-Is contract).
func TestInProcSpawnerClient_StopExternalAgent_NoSession_ReturnsError(t *testing.T) {
	_, _, client, _ := newInProcTestSetup(t)

	err := client.StopExternalAgent(context.Background(), "ses_does_not_exist")
	require.Error(t, err)
}

// TODO: test for StartChatWithStatus. The streaming handler
// `startChatSessionHandler` calls into the chat controller (LLM +
// provider validation), which is non-trivial to satisfy from
// memorystore in isolation — providers, model catalogue, controller
// scheduler, etc. The structural adapter logic (sseCapture + SSE
// parsing) is exercised end-to-end by the helix-org alpha sandbox
// flow in the inner Helix; a focused unit test belongs in the
// follow-up that stubs Controller.ChatCompletion / a fake
// startChatSessionHandler entrypoint.
