package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/go-jose/go-jose.v2"
	"gopkg.in/go-jose/go-jose.v2/jwt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	minimumPasswordLength = 8
	maximumPasswordLength = 128
)

type Notifier interface {
	Notify(ctx context.Context, n *types.Notification) error
}

type HelixAuthenticator struct {
	cfg       *config.ServerConfig
	store     store.Store
	jwtSecret []byte
	signer    jose.Signer
	notifier  Notifier // Used to send verification/password reset emails
}

type helixTokenClaims struct {
	jwt.Claims
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Admin  bool   `json:"admin"`
}

var _ Authenticator = &HelixAuthenticator{}

func NewHelixAuthenticator(cfg *config.ServerConfig, store store.Store, jwtSecret string, notifier Notifier) (*HelixAuthenticator, error) {
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
		cfg:       cfg,
		store:     store,
		jwtSecret: secret,
		signer:    signer,
		notifier:  notifier,
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

	if h.cfg.WebServer.AdminIDs != nil && slices.Contains(h.cfg.WebServer.AdminIDs, types.AdminAllUsers) {
		user.Admin = true
	}

	return h.store.CreateUser(ctx, user)
}

func (h *HelixAuthenticator) UpdatePassword(ctx context.Context, userID, newPassword string) error {
	if len(newPassword) < minimumPasswordLength || len(newPassword) > maximumPasswordLength {
		return fmt.Errorf("password must be between %d and %d characters", minimumPasswordLength, maximumPasswordLength)
	}

	user, err := h.store.GetUser(ctx, &store.GetUserQuery{ID: userID})
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.PasswordHash = hash
	user.Password = ""

	_, err = h.store.UpdateUser(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

func (h *HelixAuthenticator) RequestPasswordReset(ctx context.Context, email string) error {
	user, err := h.store.GetUser(ctx, &store.GetUserQuery{Email: email})
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if user == nil {
		return fmt.Errorf("user not found")
	}

	// Generate a JWT token for the password reset
	accessToken, err := h.GenerateUserToken(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to generate user token: %w", err)
	}

	callbackURL := fmt.Sprintf("%s/password-reset-complete?token=%s", h.cfg.WebServer.URL, accessToken)

	// Send the email for password reset
	notification := &types.Notification{
		Session:        &types.Session{Owner: user.ID}, // TODO: switch fully to user IDs, remove session
		Email:          user.Email,
		Event:          types.EventPasswordResetRequest,
		Message:        fmt.Sprintf("Click on this [link](%s) to reset your password. If you did not request a password reset, please ignore this email.", callbackURL),
		RenderMarkdown: true,
	}
	err = h.notifier.Notify(ctx, notification)
	if err != nil {
		return fmt.Errorf("failed to send password reset email: %w", err)
	}

	return nil
}

// PasswordResetComplete - called together with the email token which was sent to the user
func (h *HelixAuthenticator) PasswordResetComplete(ctx context.Context, accessToken, newPassword string) error {
	if len(newPassword) < minimumPasswordLength || len(newPassword) > maximumPasswordLength {
		return fmt.Errorf("password must be between %d and %d characters", minimumPasswordLength, maximumPasswordLength)
	}

	token, err := jwt.ParseSigned(accessToken)
	if err != nil {
		return fmt.Errorf("failed to parse token: %w", err)
	}

	var claims helixTokenClaims
	if err := token.Claims(h.jwtSecret, &claims); err != nil {
		return fmt.Errorf("failed to verify token: %w", err)
	}

	if err := claims.Validate(jwt.Expected{
		Time: time.Now(),
	}); err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	if claims.UserID == "" {
		return errors.New("invalid token: user_id claim missing or invalid")
	}

	user, err := h.store.GetUser(ctx, &store.GetUserQuery{ID: claims.UserID})
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Create a new password hash
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to generate password hash: %w", err)
	}

	user.PasswordHash = hash
	user.Password = ""

	_, err = h.store.UpdateUser(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
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

	// Check if the user is an admin
	if h.cfg.WebServer.AdminIDs != nil && slices.Contains(h.cfg.WebServer.AdminIDs, user.ID) ||
		slices.Contains(h.cfg.WebServer.AdminIDs, types.AdminAllUsers) {
		user.Admin = true
	}

	return &types.User{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		FullName:    user.FullName,
		Token:       accessToken,
		TokenType:   types.TokenTypeNone,
		Type:        types.OwnerTypeUser,
		Admin:       user.Admin,
		SB:          user.SB,
		Deactivated: user.Deactivated,
	}, nil
}

func (h *HelixAuthenticator) GenerateUserToken(_ context.Context, user *types.User) (string, error) {
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
