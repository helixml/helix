package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// The callback completes the request token named by its state, which must
// belong to the user and provider completing it, rather than whatever flow
// that user started most recently.
func newStateTestProvider(t *testing.T, tokenURL string) (*OAuth2Provider, *store.MockStore) {
	t.Helper()
	st := store.NewMockStore(gomock.NewController(t))
	p, err := NewOAuth2Provider(context.Background(), &types.OAuthProvider{
		ID: "prov-1", Name: "GitHub", Type: types.OAuthProviderTypeGitHub, Enabled: true,
		ClientID: "cid", ClientSecret: "secret",
		AuthURL: "https://example.invalid/authorize", TokenURL: tokenURL, UserInfoURL: "https://example.invalid/user",
	}, st)
	require.NoError(t, err)
	return p.(*OAuth2Provider), st
}

func TestCompleteAuthorization_StateOfAnotherUserRefused(t *testing.T) {
	p, st := newStateTestProvider(t, "https://example.invalid/token")
	st.EXPECT().GetOAuthRequestTokenByState(gomock.Any(), "their-state").Return([]*types.OAuthRequestToken{
		{ID: "req-them", UserID: "usr_other", ProviderID: "prov-1", State: "their-state", ExpiresAt: time.Now().Add(time.Minute)},
	}, nil)
	// No GetOAuthRequestToken fallback and no token exchange may happen.

	_, err := p.CompleteAuthorization(context.Background(), "usr_me", "code", "their-state")
	require.Error(t, err)
}

func TestCompleteAuthorization_StateOfAnotherProviderRefused(t *testing.T) {
	p, st := newStateTestProvider(t, "https://example.invalid/token")
	st.EXPECT().GetOAuthRequestTokenByState(gomock.Any(), "state-x").Return([]*types.OAuthRequestToken{
		{ID: "req-1", UserID: "usr_me", ProviderID: "prov-other", State: "state-x", ExpiresAt: time.Now().Add(time.Minute)},
	}, nil)

	_, err := p.CompleteAuthorization(context.Background(), "usr_me", "code", "state-x")
	require.Error(t, err)
}

func TestCompleteAuthorization_UsesExactStateToken(t *testing.T) {
	// A token endpoint that rejects the exchange: reaching it proves the
	// state-named token was selected and consumed.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer tokenSrv.Close()
	p, st := newStateTestProvider(t, tokenSrv.URL)
	st.EXPECT().GetOAuthRequestTokenByState(gomock.Any(), "my-state").Return([]*types.OAuthRequestToken{
		{ID: "req-mine", UserID: "usr_me", ProviderID: "prov-1", State: "my-state", ExpiresAt: time.Now().Add(time.Minute)},
	}, nil)
	st.EXPECT().DeleteOAuthRequestToken(gomock.Any(), "req-mine").Return(nil)

	_, err := p.CompleteAuthorization(context.Background(), "usr_me", "code", "my-state")
	require.ErrorContains(t, err, "failed to exchange code")
}

func TestCompleteAuthorization_ExpiredStateTokenRefused(t *testing.T) {
	p, st := newStateTestProvider(t, "https://example.invalid/token")
	st.EXPECT().GetOAuthRequestTokenByState(gomock.Any(), "stale").Return([]*types.OAuthRequestToken{
		{ID: "req-stale", UserID: "usr_me", ProviderID: "prov-1", State: "stale", ExpiresAt: time.Now().Add(-time.Minute)},
	}, nil)

	_, err := p.CompleteAuthorization(context.Background(), "usr_me", "code", "stale")
	require.ErrorContains(t, err, "expired")
}
