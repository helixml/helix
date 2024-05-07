package auth

import (
	"context"

	jwt "github.com/golang-jwt/jwt/v5"

	"github.com/helixml/helix/api/pkg/types"
)

type MockAuthenticator struct {
	user *types.User
}

func NewMockAuthenticator(user *types.User) *MockAuthenticator {
	return &MockAuthenticator{
		user: user,
	}
}

func (m *MockAuthenticator) GetUserByID(ctx context.Context, userID string) (*types.User, error) {
	return m.user, nil
}

func (m *MockAuthenticator) ValidateUserToken(ctx context.Context, token string) (*jwt.Token, error) {
	return nil, nil
}
