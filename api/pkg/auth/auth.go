package auth

import (
	"context"

	"github.com/coreos/go-oidc"
	"golang.org/x/oauth2"

	"github.com/helixml/helix/api/pkg/types"
)

type Authenticator interface {
	GetUserByID(ctx context.Context, userID string) (*types.User, error)
	CreateUser(ctx context.Context, user *types.User) (*types.User, error)
	ValidatePassword(ctx context.Context, user *types.User, password string) error

	// In both keycloak case and OIDC this is using OIDC client,
	// in regular auth - it's validating cookies
	ValidateUserToken(ctx context.Context, accessToken string) (*types.User, error)

	GenerateUserToken(ctx context.Context, user *types.User) (string, error)
}

type OIDC interface {
	// AuthenticatorOIDC
	ValidateUserToken(ctx context.Context, accessToken string) (*types.User, error)
	GetAuthURL(state, nonce string) string
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	VerifyIDToken(ctx context.Context, token *oauth2.Token) (*oidc.IDToken, error)
	VerifyAccessToken(ctx context.Context, accessToken string) error
	RefreshAccessToken(ctx context.Context, refreshToken string) (*oauth2.Token, error)
	GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error)
	GetLogoutURL() (string, error)
}
