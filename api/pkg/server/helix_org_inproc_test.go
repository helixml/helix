package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/pubsub"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
)

// newInProcTestSetup builds a NewTestServer-backed HelixAPIServer + an
// inProcHelixClient with a request user. The single setup
// covers every test in this file — each test seeds whatever store
// state it needs against the returned memorystore.
//
// We deliberately leave `Cfg.Inference` / providers etc unset: the
// adapter's ProjectService methods we exercise here don't touch those
// fields (`applyProject` does, but it isn't covered in this test
// suite — that handler's full happy path is exercised separately
// via the in-Helix end-to-end tests).
func newInProcTestSetup(t *testing.T) (*HelixAPIServer, *memorystore.MemoryStore, *inProcHelixClient, *types.User, context.Context) {
	t.Helper()
	store := memorystore.New()
	server := NewTestServer(store, pubsub.NewNoop())
	user := &types.User{
		ID:        "usr_request",
		Email:     "request@helix.local",
		FullName:  "Request User",
		Type:      types.OwnerTypeUser,
		TokenType: types.TokenTypeAPIKey,
	}
	client := NewInProcHelixClient(server)
	ctx := runtimehelix.WithUser(context.Background(), user)
	return server, store, client, user, ctx
}

func TestInProcClient_ResolvesOrganizationOwnerWithoutAdmin(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := helixstore.NewMockStore(ctrl)
	owner := &types.User{ID: "usr_owner", Admin: false}
	st.EXPECT().GetOrganization(gomock.Any(), &helixstore.GetOrganizationQuery{ID: "org_test"}).
		Return(&types.Organization{ID: "org_test", Owner: owner.ID}, nil)
	st.EXPECT().GetUser(gomock.Any(), &helixstore.GetUserQuery{ID: owner.ID}).Return(owner, nil)

	client := NewInProcHelixClient(&HelixAPIServer{Store: st})
	ctx := runtimehelix.WithHelixIdentity(context.Background(), runtimehelix.HelixIdentity{OrganizationID: "org_test"})
	got, err := client.resolveUser(ctx)

	require.NoError(t, err)
	require.Same(t, owner, got)
}

// TestInProcProjectService_GetProject_NotFound_ReturnsErrProjectNotFound
// exercises the 404 → ErrProjectNotFound mapping that WorkerProject.Ensure's
// stale-pointer recovery path depends on (Ensure → GetProject errors.Is(…,
// ErrProjectNotFound) → ClearProject + re-apply).
func TestInProcProjectService_GetProject_NotFound_ReturnsErrProjectNotFound(t *testing.T) {
	_, _, client, _, ctx := newInProcTestSetup(t)

	_, err := client.GetProject(ctx, "proj_missing")
	require.Error(t, err)
	require.True(t, errors.Is(err, runtimehelix.ErrProjectNotFound),
		"expected ErrProjectNotFound, got %v", err)
}

// TestInProcProjectService_GetProject_Found_ReturnsProject verifies the
// happy path — a seeded project row round-trips through the inProc
// adapter back to types.Project. Authorization passes because the
// adapter resolves the request user as the project owner.
func TestInProcProjectService_GetProject_Found_ReturnsProject(t *testing.T) {
	_, store, client, user, ctx := newInProcTestSetup(t)

	store.SeedProject(&types.Project{
		ID:     "proj_test_1",
		Name:   "test-project",
		UserID: user.ID,
		Status: "active",
	})

	got, err := client.GetProject(ctx, "proj_test_1")
	require.NoError(t, err)
	require.Equal(t, "proj_test_1", got.ID)
	require.Equal(t, "test-project", got.Name)
}

// TestInProcSpawnerClient_GetOutput_ReturnsStoreData seeds a session
// + interactions and asserts GetOutput returns the data the handler
// would have served via /api/v1/sessions/{id}/output.
func TestInProcSpawnerClient_GetOutput_ReturnsStoreData(t *testing.T) {
	_, store, client, user, ctx := newInProcTestSetup(t)

	session, err := store.CreateSession(context.Background(), types.Session{
		ID:    "ses_test_1",
		Owner: user.ID,
	})
	require.NoError(t, err)

	_, err = store.CreateInteraction(context.Background(), &types.Interaction{
		ID:              "int_test_1",
		SessionID:       session.ID,
		State:           types.InteractionStateComplete,
		ResponseMessage: "hello from the test",
	})
	require.NoError(t, err)

	out, err := client.GetOutput(ctx, "ses_test_1")
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
	_, store, client, user, ctx := newInProcTestSetup(t)

	want := types.AppConfig{
		AllowedDomains: []string{"example.test"},
		Secrets:        map[string]string{"FOO": "bar"},
		Helix: types.AppHelixConfig{
			Name: "test-app",
		},
	}
	store.SeedApp(&types.App{
		ID:     "app_test_1",
		Owner:  user.ID,
		Config: want,
	})

	got, err := client.GetAppConfig(ctx, "app_test_1")
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
	_, _, client, _, ctx := newInProcTestSetup(t)

	err := client.StopExternalAgent(ctx, "ses_does_not_exist")
	require.Error(t, err)
}

// TestInProcSpawnerClient_ClearSession_RemovesInteractionsKeepsSession
// pins the wiring the spawner relies on before every re-activation:
// ClearSession routes through clearSessionHandler (so authz matches the
// SendMessage path), wipes the session's interactions, and preserves the
// session row itself. An internal (non-Zed) session has a no-op runtime
// backend, so this exercises the shared DB clear in isolation.
func TestInProcSpawnerClient_ClearSession_RemovesInteractionsKeepsSession(t *testing.T) {
	_, store, client, user, ctx := newInProcTestSetup(t)

	session, err := store.CreateSession(context.Background(), types.Session{
		ID:    "ses_clear_1",
		Owner: user.ID,
	})
	require.NoError(t, err)
	_, err = store.CreateInteraction(context.Background(), &types.Interaction{
		ID:              "int_clear_1",
		SessionID:       session.ID,
		State:           types.InteractionStateComplete,
		ResponseMessage: "prior context",
	})
	require.NoError(t, err)

	require.NoError(t, client.ClearSession(ctx, "ses_clear_1"))

	// Session row preserved.
	got, err := store.GetSession(context.Background(), "ses_clear_1")
	require.NoError(t, err)
	require.NotNil(t, got)
	// Interactions gone — the next turn starts on an empty context.
	ints, _, err := store.ListInteractions(context.Background(), &types.ListInteractionsQuery{
		SessionID:    "ses_clear_1",
		GenerationID: -1,
	})
	require.NoError(t, err)
	require.Empty(t, ints, "ClearSession must remove the session's interactions")
}

// TestInProcSpawnerClient_ClearSession_NoSession_ReturnsError confirms a
// missing session surfaces as an error rather than silently succeeding,
// so a stale persisted session pointer fails the activation loudly.
func TestInProcSpawnerClient_ClearSession_NoSession_ReturnsError(t *testing.T) {
	_, _, client, _, ctx := newInProcTestSetup(t)

	err := client.ClearSession(ctx, "ses_does_not_exist")
	require.Error(t, err)
}

// TestParseEnvVarsToMap pins the KEY=value split that backs
// ListProjectSecrets / list_secrets. A value containing `=` (base64,
// tokens, URL query strings) must survive intact — Cut on the FIRST `=`
// — and a malformed entry must never produce a "" key.
func TestParseEnvVarsToMap(t *testing.T) {
	got := parseEnvVarsToMap([]string{
		"DRONE_TOKEN=abc",
		"B64=a=b=c==",           // value with `=` — keep everything after the first
		"URL=https://x?a=1&b=2", // query string with `=`
		"EMPTY=",                // empty value is a valid secret
		"=orphan",               // empty name — dropped
		"NOEQUALS",              // no `=` — dropped
	})
	require.Equal(t, map[string]string{
		"DRONE_TOKEN": "abc",
		"B64":         "a=b=c==",
		"URL":         "https://x?a=1&b=2",
		"EMPTY":       "",
	}, got)
}

// TODO: tests for StartSession / SendMessage. StartSession routes to
// StartExternalAgentSession (starts a real dev container) and
// SendMessage to sendSessionMessage (needs a connected external-agent
// WS), both non-trivial to satisfy from memorystore in isolation. These
// adapters are the same shared primitives the cron trigger and spec
// tasks use, and are exercised end-to-end by the helix-org alpha sandbox
// flow in the inner Helix; focused unit tests belong in a follow-up that
// stubs the executor + WS manager.
