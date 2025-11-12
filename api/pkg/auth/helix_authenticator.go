package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/go-jose/go-jose.v2"
	"gopkg.in/go-jose/go-jose.v2/jwt"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type HelixAuthenticator struct {
	store     store.Store
	jwtSecret []byte
	signer    jose.Signer
}

type helixTokenClaims struct {
	jwt.Claims
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Admin  bool   `json:"admin"`
}

var _ Authenticator = &HelixAuthenticator{}

func NewHelixAuthenticator(store store.Store, jwtSecret string) (*HelixAuthenticator, error) {
	secret := []byte(jwtSecret)
	if jwtSecret == "" {
		secret = []byte("helix-default-secret-change-in-production")
	}

	key := jose.SigningKey{
		Algorithm: jose.HS256,
		Key:       secret,
	}

	signer, err := jose.NewSigner(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWT signer: %w", err)
	}

	return &HelixAuthenticator{
		store:     store,
		jwtSecret: secret,
		signer:    signer,
	}, nil
}

func (h *HelixAuthenticator) GetUserByID(ctx context.Context, userID string) (*types.User, error) {
	return h.store.GetUser(ctx, &store.GetUserQuery{ID: userID})
}

func (h *HelixAuthenticator) CreateUser(ctx context.Context, user *types.User) (*types.User, error) {
	if user.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		user.PasswordHash = hash
		user.Password = ""
	}

	return h.store.CreateUser(ctx, user)
}

func (h *HelixAuthenticator) ValidatePassword(_ context.Context, user *types.User, password string) error {
	return bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password))
}

func (h *HelixAuthenticator) ValidateUserToken(ctx context.Context, accessToken string) (*types.User, error) {
	if accessToken == "" {
		return nil, errors.New("access token is required")
	}

	token, err := jwt.ParseSigned(accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	var claims helixTokenClaims
	if err := token.Claims(h.jwtSecret, &claims); err != nil {
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}

	if err := claims.Validate(jwt.Expected{
		Time: time.Now(),
	}); err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	if claims.UserID == "" {
		return nil, errors.New("invalid token: user_id claim missing or invalid")
	}

	user, err := h.store.GetUser(ctx, &store.GetUserQuery{ID: claims.UserID})
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &types.User{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		FullName:    user.FullName,
		Token:       accessToken,
		TokenType:   types.TokenTypeNone,
		Type:        types.OwnerTypeUser,
		Admin:       claims.Admin,
		SB:          user.SB,
		Deactivated: user.Deactivated,
	}, nil
}

func (h *HelixAuthenticator) GenerateUserToken(ctx context.Context, user *types.User) (string, error) {
	if user == nil {
		return "", errors.New("user is required")
	}

	now := time.Now()
	expiry := now.Add(24 * time.Hour)

	claims := jwt.Claims{
		Subject:   user.ID,
		Issuer:    "helix",
		Expiry:    jwt.NewNumericDate(expiry),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
	}

	customClaims := map[string]interface{}{
		"user_id": user.ID,
		"email":   user.Email,
		"admin":   user.Admin,
	}

	builder := jwt.Signed(h.signer).Claims(claims).Claims(customClaims)
	tokenString, err := builder.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize token: %w", err)
	}

	return tokenString, nil
}
