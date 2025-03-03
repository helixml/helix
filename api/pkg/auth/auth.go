package auth

import (
	"context"

	jwt "github.com/golang-jwt/jwt/v5"

	"github.com/helixml/helix/api/pkg/types"
)

type Authenticator interface {
	GetUserByID(ctx context.Context, userID string) (*types.User, error)
	ValidateUserToken(ctx context.Context, token string) (*jwt.Token, error)
	// SearchUsers searches for users by search term (matches against email, username, or full name)
	SearchUsers(ctx context.Context, searchTerm string) ([]*types.User, error)
}
