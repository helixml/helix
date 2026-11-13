package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	encryptionKeyA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	encryptionKeyB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	staleSecretPlaintext   = "stale-plaintext-0xDEADBEEF"
	currentSecretPlaintext = "current-plaintext-0xC0FFEE"

	undecryptableProject = "prj_secrets"
)

func encryptUnder(t *testing.T, hexKey, plaintext string) []byte {
	t.Helper()
	t.Setenv("HELIX_ENCRYPTION_KEY", hexKey)
	key, err := crypto.GetEncryptionKey()
	require.NoError(t, err)
	ciphertext, err := crypto.EncryptAES256GCM([]byte(plaintext), key)
	require.NoError(t, err)
	return []byte(ciphertext)
}

// projectSecretsUnderRotatedKey returns one secret written under key A and one
// under key B, and leaves the process running with key B.
func projectSecretsUnderRotatedKey(t *testing.T) []*types.Secret {
	t.Helper()
	stale := encryptUnder(t, encryptionKeyA, staleSecretPlaintext)
	current := encryptUnder(t, encryptionKeyB, currentSecretPlaintext)
	return []*types.Secret{
		{ID: "sec_stale", Name: "PENTEST_TOKEN", Value: stale, ProjectID: undecryptableProject, Scope: types.SecretScopeDev},
		{ID: "sec_current", Name: "API_URL", Value: current, ProjectID: undecryptableProject, Scope: types.SecretScopeBoth},
	}
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&buf).Level(zerolog.DebugLevel)
	t.Cleanup(func() { log.Logger = previous })
	return &buf
}

func TestGetProjectSecretsAsEnvVarsReportsUndecryptableSecrets(t *testing.T) {
	secrets := projectSecretsUnderRotatedKey(t)
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListProjectSecrets(gomock.Any(), undecryptableProject).Return(secrets, nil)
	server := &HelixAPIServer{Store: mockStore}
	logs := captureLogs(t)

	got, err := server.GetProjectSecretsAsEnvVars(context.Background(), undecryptableProject, types.SecretScopeDev)
	require.NoError(t, err)

	require.Equal(t, []string{"API_URL=" + currentSecretPlaintext}, got.Env)
	require.Equal(t, []string{"PENTEST_TOKEN"}, got.Undecryptable)
	require.Contains(t, logs.String(), "PENTEST_TOKEN")
	require.NotContains(t, logs.String(), staleSecretPlaintext)
	require.NotContains(t, logs.String(), string(secrets[0].Value))
}

func TestGetProjectSecretsAsEnvVarsIgnoresUndecryptableSecretOutsideTarget(t *testing.T) {
	secrets := projectSecretsUnderRotatedKey(t)
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListProjectSecrets(gomock.Any(), undecryptableProject).Return(secrets, nil)
	server := &HelixAPIServer{Store: mockStore}

	got, err := server.GetProjectSecretsAsEnvVars(context.Background(), undecryptableProject, types.SecretScopeProd)
	require.NoError(t, err)
	require.Equal(t, []string{"API_URL=" + currentSecretPlaintext}, got.Env)
	require.Empty(t, got.Undecryptable)
}

func listProjectSecretsRequest(user types.User) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+undecryptableProject+"/secrets", nil)
	req = mux.SetURLVars(req, map[string]string{"id": undecryptableProject})
	return req.WithContext(setRequestUser(req.Context(), user))
}

func TestListProjectSecretsFlagsUndecryptableWithoutValues(t *testing.T) {
	secrets := projectSecretsUnderRotatedKey(t)
	ciphertexts := []string{string(secrets[0].Value), string(secrets[1].Value)}
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetProject(gomock.Any(), undecryptableProject).
		Return(&types.Project{ID: undecryptableProject, UserID: "user_owner"}, nil)
	mockStore.EXPECT().ListProjectSecrets(gomock.Any(), undecryptableProject).Return(secrets, nil)
	server := &HelixAPIServer{Store: mockStore}
	logs := captureLogs(t)

	got, httpErr := server.listProjectSecrets(httptest.NewRecorder(), listProjectSecretsRequest(types.User{ID: "user_owner"}))
	require.Nil(t, httpErr)
	require.Len(t, got, 2)
	require.True(t, got[0].Undecryptable)
	require.False(t, got[1].Undecryptable)

	body, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(body), `"undecryptable":true`)
	for _, leak := range append(ciphertexts, staleSecretPlaintext, currentSecretPlaintext) {
		require.NotContains(t, string(body), leak)
		require.NotContains(t, logs.String(), leak)
	}
}

// TestListProjectSecretsRefusesOtherTenants: the list handler now decrypts
// every value to flag stale ones, so a caller outside the project must be
// refused before the secrets are even loaded.
func TestListProjectSecretsRefusesOtherTenants(t *testing.T) {
	for _, tc := range []struct {
		name string
		user types.User
	}{
		{"owner of another org", types.User{ID: tenantBAdmin}},
		{"member of another org", types.User{ID: tenantBUser}},
		{"org member without a project grant", types.User{ID: tenantAUser}},
		{"org key scoped to another org", types.User{ID: tenantAAdmin, OrganizationID: tenantBOrg}},
		{"key scoped to another project", types.User{ID: tenantAAdmin, ProjectID: "prj_other"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HELIX_ENCRYPTION_KEY", encryptionKeyB)
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			all := membershipsByUser()
			mockStore.EXPECT().GetProject(gomock.Any(), undecryptableProject).
				Return(&types.Project{ID: undecryptableProject, UserID: tenantAAdmin, OrganizationID: tenantAOrg}, nil)
			mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, q *store.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
					for _, m := range all[q.UserID] {
						if m.OrganizationID == q.OrganizationID {
							return m, nil
						}
					}
					return nil, store.ErrNotFound
				}).AnyTimes()
			mockStore.EXPECT().ListTeams(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			mockStore.EXPECT().ListAccessGrants(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			// No ListProjectSecrets expectation: gomock fails the test if
			// the handler loads the secrets for a refused caller.
			server := &HelixAPIServer{Store: mockStore}

			got, httpErr := server.listProjectSecrets(httptest.NewRecorder(), listProjectSecretsRequest(tc.user))
			require.NotNil(t, httpErr)
			require.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, httpErr.StatusCode)
			require.Nil(t, got)
		})
	}
}

func TestReportUndecryptableProjectSecretsRaisesAttentionEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{ID: "spt_1", ProjectID: undecryptableProject, Name: "Retest"}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), undecryptableProject).
		Return(&types.Project{ID: undecryptableProject, UserID: "user_owner", OrganizationID: "org_1", Name: "proj"}, nil)
	var created *types.AttentionEvent
	mockStore.EXPECT().CreateAttentionEvent(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, event *types.AttentionEvent) (*types.AttentionEvent, error) {
			created = event
			return event, nil
		})
	slackLookup := make(chan struct{})
	mockStore.EXPECT().ListApps(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *store.ListAppsQuery) ([]*types.App, error) {
			close(slackLookup)
			return nil, nil
		})
	server := &HelixAPIServer{Store: mockStore, attentionService: services.NewAttentionService(mockStore, nil)}

	server.reportUndecryptableProjectSecrets(context.Background(),
		&types.DesktopAgent{SpecTaskID: task.ID, ProjectID: undecryptableProject},
		[]string{"PENTEST_TOKEN", "OLD_KEY"})

	<-slackLookup
	require.NotNil(t, created)
	require.Equal(t, types.AttentionEventProjectSecretUndecryptable, created.EventType)
	require.Equal(t, "user_owner", created.UserID)
	require.Equal(t, "spt_1:project_secret_undecryptable:OLD_KEY,PENTEST_TOKEN", created.IdempotencyKey)
	require.JSONEq(t, `{"secret_names":["OLD_KEY","PENTEST_TOKEN"]}`, string(created.Metadata))
	require.True(t, strings.Contains(created.Description, "re-enter"))
}

func TestReportUndecryptableProjectSecretsSkipsDesktopWithoutTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore, attentionService: services.NewAttentionService(mockStore, nil)}

	server.reportUndecryptableProjectSecrets(context.Background(),
		&types.DesktopAgent{ProjectID: undecryptableProject}, []string{"PENTEST_TOKEN"})
}
