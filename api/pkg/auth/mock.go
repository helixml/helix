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

func (m *MockAuthenticator) GetUserByID(_ context.Context, _ string) (*types.User, error) {
	return m.user, nil
}

func (m *MockAuthenticator) ValidateUserToken(_ context.Context, _ string) (*jwt.Token, error) {
	return nil, nil
}
