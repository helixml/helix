package auth

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/types"
)

var (
	ErrNoUserIDFound = errors.New("no user ID found")
)

type Authenticator interface {
	ValidateAndReturnUser(ctx context.Context, token string) (*types.User, error)
}

type OIDCAuthenticator interface {
	ValidateAndReturnUser(ctx context.Context, token string) (*types.User, error)
}

type RunnerTokenAuthenticator struct {
	RunnerToken string
}

func (authenticator *RunnerTokenAuthenticator) ValidateAndReturnUser(ctx context.Context, token string) (*types.User, error) {
	if token == authenticator.RunnerToken {
		return &types.User{
			Token:     token,
			TokenType: types.TokenTypeRunner,
		}, nil
	}
	return nil, nil
}

// Compile-time interface check:
var _ Authenticator = (*RunnerTokenAuthenticator)(nil)
