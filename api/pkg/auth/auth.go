package auth

import (
	"context"

	"github.com/coreos/go-oidc"
	jwt "github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"

	"github.com/helixml/helix/api/pkg/types"
)

type Authenticator interface {
	GetUserByID(ctx context.Context, userID string) (*types.User, error)
	ValidateUserToken(ctx context.Context, token string) (*jwt.Token, error)
}

type AuthenticatorOIDC interface {
	ValidateUserToken(ctx context.Context, accessToken string) (*types.User, error)
}

type OIDC interface {
	AuthenticatorOIDC
	GetAuthURL(state, nonce string) string
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	VerifyIDToken(ctx context.Context, token *oauth2.Token) (*oidc.IDToken, error)
	VerifyAccessToken(ctx context.Context, accessToken string) error
	RefreshAccessToken(ctx context.Context, refreshToken string) (*oauth2.Token, error)
	GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error)
	GetLogoutURL() (string, error)
}
