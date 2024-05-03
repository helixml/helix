package auth

import (
	"context"

	jwt "github.com/golang-jwt/jwt/v5"

	"github.com/helixml/helix/api/pkg/types"
)

type Authenticator interface {
	GetUserByID(ctx context.Context, userID string) (*types.UserDetails, error)
	ValidateUserToken(ctx context.Context, token string) (*jwt.Token, error)
}
