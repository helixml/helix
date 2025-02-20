package auth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

type OIDCAuthenticator struct {
	cfg      *config.OIDC
	verifier *oidc.IDTokenVerifier

	adminConfig *AdminConfig
}

func NewOIDCAuthenticator(cfg *config.OIDC, adminConfig *AdminConfig) (*OIDCAuthenticator, error) {
	ctx := context.Background()

	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize OIDC provider: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.APIClientID})

	return &OIDCAuthenticator{
		cfg: cfg,
		verifier: verifier,
		adminConfig: adminConfig,
	}, nil
}

func (o *OIDCAuthenticator) ValidateAndReturnUser(ctx context.Context, tokenString string) (*types.User, error) {
	token, err := o.verifier.Verify(ctx, tokenString)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	var claims jwt.MapClaims
	if err := token.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse token claims: %w", err)
	}

	sub, err := claims.GetSubject()
	if err != nil {
		return nil, fmt.Errorf("subject missing from token: %w", err)
	}

	acc := account{
		token: &tokenAcct{claims: claims, userID: sub},
	}

	return &types.User{
		ID:        sub,
		Username:  sub,
		Email:     claims["email"].(string),
		FullName:  claims["full_name"].(string),
		Token:     tokenString,
		TokenType: types.TokenTypeKeycloak,
		Type:      types.OwnerTypeUser,
		Admin:     acc.isAdmin(o.adminConfig),
	}, nil
}

// Compile-time interface check:
var _ Authenticator = (*OIDCAuthenticator)(nil)
var _ BearerAuthenticator = (*OIDCAuthenticator)(nil)
