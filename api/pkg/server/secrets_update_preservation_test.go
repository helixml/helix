package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// The store persists an updated secret with a full-row Save, so every field a
// PUT body omits used to be written back as its zero value: an omitted scope
// silently zeroed the injection scope, an omitted value blanked the stored
// ciphertext, and omitted name/created were wiped too. The handler must carry
// those over from the stored row.

func putSecret(t *testing.T, server *HelixAPIServer, id string, body string) (*types.Secret, *system.HTTPError) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/secrets/"+id, bytes.NewBufferString(body))
	req = mux.SetURLVars(req, map[string]string{"id": id})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user_test"}))
	return server.updateSecret(httptest.NewRecorder(), req)
}

func TestUpdateSecretOmittedFieldsPreserved(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	created := time.Now().Add(-48 * time.Hour).UTC().Truncate(time.Second)
	existing := &types.Secret{
		ID: "sec_test", Owner: "user_test", OwnerType: types.OwnerTypeUser,
		Name: "OLD_NAME", Scope: types.SecretScopeProd, Created: created,
		Value: []byte("stored-ciphertext-A"),
	}
	mockStore.EXPECT().GetSecret(gomock.Any(), "sec_test").Return(existing, nil)

	var saved *types.Secret
	mockStore.EXPECT().UpdateSecret(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, s *types.Secret) (*types.Secret, error) {
			cp := *s
			saved = &cp
			return s, nil
		})

	// PUT with only a new name: no value, no scope, no created.
	updated, httpErr := putSecret(t, server, "sec_test", `{"name":"NEW_NAME"}`)
	require.Nil(t, httpErr)
	require.NotNil(t, saved)

	require.Equal(t, "NEW_NAME", saved.Name)
	require.Equal(t, types.SecretScopeProd, saved.Scope, "omitted scope must keep the stored scope")
	require.Equal(t, []byte("stored-ciphertext-A"), saved.Value, "omitted value must keep the stored ciphertext")
	require.True(t, saved.Created.Equal(created), "omitted created must keep the stored creation time")
	require.Equal(t, "user_test", saved.Owner)
	require.Equal(t, types.OwnerTypeUser, saved.OwnerType)
	require.Empty(t, updated.Value, "response must not leak the value")
}

func TestUpdateSecretRotatesValueAndPreservesScope(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	existing := &types.Secret{
		ID: "sec_test", Owner: "user_test", OwnerType: types.OwnerTypeUser,
		Name: "TOKEN", Scope: types.SecretScopeDev, Created: time.Now().Add(-time.Hour),
		Value: []byte("stored-ciphertext-A"),
	}
	mockStore.EXPECT().GetSecret(gomock.Any(), "sec_test").Return(existing, nil)

	var saved *types.Secret
	mockStore.EXPECT().UpdateSecret(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, s *types.Secret) (*types.Secret, error) {
			cp := *s
			saved = &cp
			return s, nil
		})

	// []byte fields travel base64-encoded in JSON: "dG9wc2VjcmV0" is "topsecret".
	_, httpErr := putSecret(t, server, "sec_test", `{"value":"dG9wc2VjcmV0"}`)
	require.Nil(t, httpErr)
	require.NotNil(t, saved)

	require.Equal(t, "TOKEN", saved.Name, "omitted name must keep the stored name")
	require.Equal(t, types.SecretScopeDev, saved.Scope)
	require.NotEqual(t, []byte("stored-ciphertext-A"), saved.Value, "provided value must replace the stored one")
	require.NotEqual(t, []byte("topsecret"), saved.Value, "the stored value must not be plaintext")

	key, err := crypto.GetEncryptionKey()
	require.NoError(t, err)
	plaintext, err := crypto.DecryptAES256GCM(string(saved.Value), key)
	require.NoError(t, err, "rotated value must decrypt under the server key")
	require.Equal(t, "topsecret", string(plaintext))
}

func TestUpdateSecretProjectScopedPreservesScope(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	// Old-style user-owned project (no organization): the project owner is
	// authorized to rotate secrets in it.
	existing := &types.Secret{
		ID: "sec_proj", Owner: "user_test", OwnerType: types.OwnerTypeUser,
		Name: "MP_PASSWORD", ProjectID: "prj_1", Scope: types.SecretScopeProd,
		Value: []byte("stored-ciphertext-A"),
	}
	mockStore.EXPECT().GetSecret(gomock.Any(), "sec_proj").Return(existing, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_1").Return(&types.Project{ID: "prj_1", UserID: "user_test"}, nil)

	var saved *types.Secret
	mockStore.EXPECT().UpdateSecret(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, s *types.Secret) (*types.Secret, error) {
			cp := *s
			saved = &cp
			return s, nil
		})

	_, httpErr := putSecret(t, server, "sec_proj", `{"name":"MP_PASSWORD","value":"bmV3LXNlY3JldA=="}`)
	require.Nil(t, httpErr)
	require.NotNil(t, saved)

	require.Equal(t, "prj_1", saved.ProjectID)
	require.Equal(t, types.SecretScopeProd, saved.Scope, "scope must survive a value rotation on a project secret")
}
