package controller

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// evaluateAs resolves an app whose header references one app-scoped secret
// (APP_TOKEN) and one unscoped personal secret (PERSONAL_TOKEN) of its owner,
// as the given caller. The caller also holds same-named secrets of their own.
func evaluateAs(t *testing.T, callerID string) map[string]string {
	t.Helper()
	t.Setenv("HELIX_ENCRYPTION_KEY", "test-key")
	key, err := crypto.GetEncryptionKey()
	require.NoError(t, err)
	encrypt := func(v string) []byte {
		enc, err := crypto.EncryptAES256GCM([]byte(v), key)
		require.NoError(t, err)
		return []byte(enc)
	}

	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListSecrets(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, q *store.ListSecretsQuery) ([]*types.Secret, error) {
			switch q.Owner {
			case "usr_author":
				return []*types.Secret{
					{Name: "APP_TOKEN", AppID: "app_shared", Value: encrypt("author-app-token")},
					{Name: "PERSONAL_TOKEN", Value: encrypt("author-personal-token")},
					{Name: "OTHER_APP", AppID: "app_other", Value: encrypt("author-other-app")},
				}, nil
			default:
				return []*types.Secret{
					{Name: "APP_TOKEN", Value: encrypt("caller-token")},
					{Name: "PERSONAL_TOKEN", Value: encrypt("caller-token")},
				}, nil
			}
		}).AnyTimes()

	app := &types.App{
		ID:    "app_shared",
		Owner: "usr_author",
		Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			Name: "main",
			APIs: []types.AssistantAPI{{
				Name: "api",
				URL:  "https://api.example.com",
				Headers: map[string]string{
					"X-App":      "${APP_TOKEN}",
					"X-Personal": "${PERSONAL_TOKEN}",
					"X-Other":    "${OTHER_APP}",
				},
			}},
		}}}},
	}

	c := &Controller{Options: Options{Store: mockStore}}
	enriched, err := c.evaluateSecrets(context.Background(), &types.User{ID: callerID}, app)
	require.NoError(t, err)
	return enriched.Config.Helix.Assistants[0].APIs[0].Headers
}

func TestEvaluateSecrets_OwnerResolvesAppScopedAndPersonal(t *testing.T) {
	headers := evaluateAs(t, "usr_author")
	require.Equal(t, "author-app-token", headers["X-App"])
	require.Equal(t, "author-personal-token", headers["X-Personal"])
	require.NotContains(t, headers["X-Other"], "author-other-app")
}

// Another caller (a user of a shared app, or an editor who is not the owner)
// gets the secrets the owner scoped to this app, never the owner's personal
// secrets and never their own.
func TestEvaluateSecrets_OtherCallerResolvesOnlyAppScoped(t *testing.T) {
	headers := evaluateAs(t, "usr_editor")
	require.Equal(t, "author-app-token", headers["X-App"])
	require.NotContains(t, headers["X-Personal"], "author-personal-token")
	require.NotContains(t, headers["X-Personal"], "caller-token")
	require.NotContains(t, headers["X-App"], "caller-token")
}

// A secret written under an earlier encryption key is not substituted: its
// reference resolves to nothing, and neither its ciphertext nor any plaintext
// reaches the app config or the logs. The log names the secret.
func TestEvaluateSecrets_UndecryptableSecretSkipped(t *testing.T) {
	encryptUnder := func(hexKey, plaintext string) []byte {
		t.Setenv("HELIX_ENCRYPTION_KEY", hexKey)
		key, err := crypto.GetEncryptionKey()
		require.NoError(t, err)
		enc, err := crypto.EncryptAES256GCM([]byte(plaintext), key)
		require.NoError(t, err)
		return []byte(enc)
	}
	stale := encryptUnder(strings.Repeat("a", 64), "stale-plaintext-0xDEADBEEF")
	current := encryptUnder(strings.Repeat("b", 64), "current-plaintext-0xC0FFEE")

	var logs bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&logs).Level(zerolog.DebugLevel)
	t.Cleanup(func() { log.Logger = previous })

	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListSecrets(gomock.Any(), gomock.Any()).Return([]*types.Secret{
		{Name: "STALE_TOKEN", AppID: "app_1", Value: stale},
		{Name: "CURRENT_TOKEN", AppID: "app_1", Value: current},
	}, nil)
	app := &types.App{
		ID:    "app_1",
		Owner: "usr_author",
		Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			Name:         "main",
			SystemPrompt: "token: ${STALE_TOKEN}",
			APIs: []types.AssistantAPI{{
				Name:    "api",
				URL:     "https://api.example.com",
				Headers: map[string]string{"X-Stale": "${STALE_TOKEN}", "X-Current": "${CURRENT_TOKEN}"},
			}},
		}}}},
	}

	c := &Controller{Options: Options{Store: mockStore}}
	enriched, err := c.evaluateSecrets(context.Background(), &types.User{ID: "usr_author"}, app)
	require.NoError(t, err)

	assistant := enriched.Config.Helix.Assistants[0]
	require.Equal(t, "current-plaintext-0xC0FFEE", assistant.APIs[0].Headers["X-Current"])
	require.Empty(t, assistant.APIs[0].Headers["X-Stale"])
	require.Equal(t, "token: ", assistant.SystemPrompt)

	rendered, err := yaml.Marshal(enriched)
	require.NoError(t, err)
	for _, leak := range []string{string(stale), "stale-plaintext-0xDEADBEEF"} {
		require.NotContains(t, string(rendered), leak)
		require.NotContains(t, logs.String(), leak)
	}
	require.NotContains(t, logs.String(), "current-plaintext-0xC0FFEE")
	require.Contains(t, logs.String(), "STALE_TOKEN")
}
