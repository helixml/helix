package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/oauth"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// OAuth tokens for a tool run come from the calling user's own connections.
// Another user's session on an app must never borrow the app owner's tokens.
func TestResolveOAuthTokens_UsesCallerNotAppOwner(t *testing.T) {
	st := store.NewMockStore(gomock.NewController(t))
	strategy, err := NewChainStrategy(&config.ServerConfig{}, st, nil)
	require.NoError(t, err)
	strategy.SetOAuthManager(oauth.NewManager(st, false))
	strategy.SetStores(st, st)

	tool := &types.Tool{Name: "gh", ToolType: types.ToolTypeAPI, Config: types.ToolConfig{API: &types.ToolAPIConfig{OAuthProvider: "github"}}}

	st.EXPECT().GetApp(gomock.Any(), "app-1").Return(&types.App{ID: "app-1", Owner: "usr_owner"}, nil)
	st.EXPECT().GetSession(gomock.Any(), "sess-1").Return(&types.Session{ID: "sess-1", Owner: "usr_caller", ParentApp: "app-1"}, nil).AnyTimes()
	st.EXPECT().ListOAuthProviders(gomock.Any(), gomock.Any()).Return([]*types.OAuthProvider{{ID: "prov-1", Name: "github", Enabled: true}}, nil)
	// Only the caller's connections may be listed; a lookup for usr_owner is an unexpected call.
	st.EXPECT().ListOAuthConnections(gomock.Any(), &store.ListOAuthConnectionsQuery{UserID: "usr_caller", ProviderID: "prov-1"}).
		Return([]*types.OAuthConnection{{ID: "conn-caller", UserID: "usr_caller", ProviderID: "prov-1", AccessToken: "caller-token-1234567890"}}, nil)

	ctx := oai.SetContextAppID(context.Background(), "app-1")
	tokens := strategy.resolveOAuthTokens(ctx, "sess-1", tool, strategy.getDefaultOptions())

	require.Equal(t, map[string]string{"github": "caller-token-1234567890"}, tokens)
}

func TestResolveOAuthTokens_NoCallerNoTokens(t *testing.T) {
	st := store.NewMockStore(gomock.NewController(t))
	strategy, err := NewChainStrategy(&config.ServerConfig{}, st, nil)
	require.NoError(t, err)
	strategy.SetOAuthManager(oauth.NewManager(st, false))
	strategy.SetStores(st, st)

	tool := &types.Tool{Name: "gh", ToolType: types.ToolTypeAPI, Config: types.ToolConfig{API: &types.ToolAPIConfig{OAuthProvider: "github"}}}
	// No session and no request owner: nothing identifies a caller, so the
	// app owner's connections must not be consulted (no store calls allowed).
	ctx := oai.SetContextAppID(context.Background(), "app-1")
	tokens := strategy.resolveOAuthTokens(ctx, "", tool, strategy.getDefaultOptions())

	require.Empty(t, tokens)
}
