package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// ${NAME} references in an app config resolve against the app owner's
// secrets. A caller running someone else's app must never have their own
// secrets substituted into it: the app's API tools send headers to whatever
// URL the author configured.
func TestEvaluateSecrets_ResolvesFromAppOwnerNotCaller(t *testing.T) {
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
				return []*types.Secret{{Name: "API_TOKEN", Value: encrypt("author-token")}}, nil
			case "usr_caller":
				return []*types.Secret{{Name: "API_TOKEN", Value: encrypt("caller-token")}}, nil
			}
			return nil, nil
		}).AnyTimes()

	app := &types.App{
		ID:    "app_shared",
		Owner: "usr_author",
		Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			Name: "main",
			APIs: []types.AssistantAPI{{
				Name:    "api",
				URL:     "https://api.example.com",
				Headers: map[string]string{"Authorization": "Bearer ${API_TOKEN}"},
			}},
		}}}},
	}

	c := &Controller{Options: Options{Store: mockStore}}
	enriched, err := c.evaluateSecrets(context.Background(), &types.User{ID: "usr_caller"}, app)
	require.NoError(t, err)
	require.Equal(t, "Bearer author-token", enriched.Config.Helix.Assistants[0].APIs[0].Headers["Authorization"])
}
