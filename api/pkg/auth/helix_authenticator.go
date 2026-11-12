package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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

	// passwordResetTokenValidity bounds how long an emailed reset link works.
	passwordResetTokenValidity = time.Hour
	// tokenPurposePasswordReset marks a token that may only complete a
	// password reset. Session tokens carry no purpose.
	tokenPurposePasswordReset = "password_reset"
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
	// Purpose is empty for session tokens and set for single-purpose tokens,
	// which are never accepted as a session.
	Purpose string `json:"purpose,omitempty"`
	// PasswordBinding ties a password-reset token to the password hash it was
	// issued against, so it stops working once any password change lands.
	PasswordBinding string `json:"pwb,omitempty"`
}

var _ Authenticator = &HelixAuthenticator{}

// NewHelixAuthenticator creates an authenticator that signs and verifies
// tokens with jwtSecret. The secret must be private to this deployment: an
// empty secret or a value that once shipped as a public default is refused.
// Servers obtain it from ResolveJWTSecret.
func NewHelixAuthenticator(cfg *config.ServerConfig, store store.Store, jwtSecret string, notifier Notifier) (*HelixAuthenticator, error) {
	if jwtSecret == "" {
		return nil, errors.New("JWT secret is required")
	}
	if isLegacyPublicJWTSecret(jwtSecret) {
		return nil, errors.New("JWT secret is a publicly known value; set REGULAR_AUTH_JWT_SECRET to a private random value or leave it unset")
	}
	secret := []byte(jwtSecret)

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

// isUserInAdminList checks if the user ID is in the ADMIN_USER_IDS list.
// Returns true if "all" is in the list (everyone is admin) or if the specific user ID is listed.
func (h *HelixAuthenticator) isUserInAdminList(userID string) bool {
	for _, adminID := range h.cfg.WebServer.AdminUserIDs {
		if adminID == config.AdminAllUsers {
			return true // "all" means everyone is admin
		}
		if adminID == userID {
			return true
		}
	}
	return false
}

// isAllUsersAdmin reports whether ADMIN_USER_IDS contains the "all" dev-mode
// wildcard.
func (h *HelixAuthenticator) isAllUsersAdmin() bool {
	for _, adminID := range h.cfg.WebServer.AdminUserIDs {
		if adminID == config.AdminAllUsers {
			return true
		}
	}
	return false
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

	user.Waitlisted = h.cfg.Auth.Waitlist

	// Check if user should be admin based on ADMIN_USER_IDS env var.
	//
	// Only an explicit user-ID match is persisted. The "all" wildcard is a
	// dev-mode switch that already grants admin dynamically on every
	// authenticated request, so persisting it is redundant — and harmful:
	// the stored flag is never re-evaluated, so every account that registered
	// while the wildcard was on would stay a permanent superuser (treated as
	// owner of every organization, see orgstore.Authorizer.AuthorizeOrgMember)
	// long after ADMIN_USER_IDS is tightened.
	if h.isUserInAdminList(user.ID) {
		user.Waitlisted = false
		if !h.isAllUsersAdmin() {
			user.Admin = true
		}
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

	accessToken, err := h.generatePasswordResetToken(user, time.Now())
	if err != nil {
		return fmt.Errorf("failed to generate password reset token: %w", err)
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

// passwordBinding derives a value from the user's current password hash that
// changes whenever the password does. It is keyed with the signing secret so
// the token reveals nothing about the hash.
func (h *HelixAuthenticator) passwordBinding(user *types.User) string {
	mac := hmac.New(sha256.New, h.jwtSecret)
	mac.Write([]byte("password-reset\x00"))
	mac.Write([]byte(user.ID))
	mac.Write([]byte{0})
	mac.Write(user.PasswordHash)
	return hex.EncodeToString(mac.Sum(nil))
}

// generatePasswordResetToken issues a short-lived token that can only
// complete a password reset, once, for the password it was issued against.
func (h *HelixAuthenticator) generatePasswordResetToken(user *types.User, now time.Time) (string, error) {
	claims := helixTokenClaims{
		Claims: jwt.Claims{
			Subject:   user.ID,
			Issuer:    "helix",
			Expiry:    jwt.NewNumericDate(now.Add(passwordResetTokenValidity)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
		UserID:          user.ID,
		Purpose:         tokenPurposePasswordReset,
		PasswordBinding: h.passwordBinding(user),
	}
	token, err := jwt.Signed(h.signer).Claims(claims).CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize token: %w", err)
	}
	return token, nil
}

// PasswordResetComplete sets a new password using a token from
// RequestPasswordReset and returns the updated user. Session tokens are
// refused, and a reset token stops working once the password has changed.
func (h *HelixAuthenticator) PasswordResetComplete(ctx context.Context, accessToken, newPassword string) (*types.User, error) {
	if len(newPassword) < minimumPasswordLength || len(newPassword) > maximumPasswordLength {
		return nil, fmt.Errorf("password must be between %d and %d characters", minimumPasswordLength, maximumPasswordLength)
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

	if claims.Purpose != tokenPurposePasswordReset {
		return nil, errors.New("invalid token: not a password reset token")
	}

	if claims.UserID == "" {
		return nil, errors.New("invalid token: user_id claim missing or invalid")
	}

	user, err := h.store.GetUser(ctx, &store.GetUserQuery{ID: claims.UserID})
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if !hmac.Equal([]byte(claims.PasswordBinding), []byte(h.passwordBinding(user))) {
		return nil, errors.New("invalid token: password reset link has already been used or the password has changed")
	}

	// Create a new password hash
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to generate password hash: %w", err)
	}

	user.PasswordHash = hash
	user.Password = ""
	// The reset link went to the account's address, so completing it shows
	// the caller controls that mailbox.
	user.EmailVerified = true

	user, err = h.store.UpdateUser(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	if h.isUserInAdminList(user.ID) {
		user.Admin = true
	}

	return user, nil
}

func (h *HelixAuthenticator) ValidatePassword(_ context.Context, user *types.User, password string) error {
	return bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password))
}

// passwordTimingDecoyHash is a bcrypt hash (default cost, as CreateUser uses)
// of a random value that was discarded. It matches no password.
const passwordTimingDecoyHash = "$2a$10$cIVXlgpj4dPyFn1PbbvDKOBrgoAANLXJe3RP//.UzEkzKWlOauBjG"

// PasswordTimingDecoy returns an account that no password matches, for a
// login attempt against an address that has no account: validating against
// it costs the same bcrypt work as a real attempt, so the response time does
// not tell the two apart.
func PasswordTimingDecoy() *types.User {
	return &types.User{PasswordHash: []byte(passwordTimingDecoyHash)}
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

	// Single-purpose tokens (password reset) are never a session.
	if claims.Purpose != "" {
		return nil, errors.New("invalid token: not a session token")
	}

	if claims.UserID == "" {
		return nil, errors.New("invalid token: user_id claim missing or invalid")
	}

	user, err := h.store.GetUser(ctx, &store.GetUserQuery{ID: claims.UserID})
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// A session token bound to the credentials it was issued under stops
	// working once the password changes. Tokens issued before the binding
	// existed carry none and are accepted until they expire.
	if claims.PasswordBinding != "" && !hmac.Equal([]byte(claims.PasswordBinding), []byte(h.passwordBinding(user))) {
		return nil, errors.New("invalid token: credentials have changed since it was issued")
	}

	// Check admin status:
	// - If user ID is in ADMIN_USER_IDS list (or "all" is set): admin
	// - Otherwise: use database admin field
	if h.isUserInAdminList(user.ID) {
		user.Admin = true
	}

	user.Token = accessToken
	user.TokenType = types.TokenTypeNone
	user.Type = types.OwnerTypeUser

	return user, nil
}

func (h *HelixAuthenticator) GenerateUserToken(_ context.Context, user *types.User) (string, error) {
	if user == nil {
		return "", errors.New("user is required")
	}

	now := time.Now()
	expiry := now.Add(h.cfg.Auth.Regular.TokenValidity)

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
		// Bound to the current credentials: see ValidateUserToken.
		"pwb": h.passwordBinding(user),
	}

	builder := jwt.Signed(h.signer).Claims(claims).Claims(customClaims)
	tokenString, err := builder.CompactSerialize()
	if err != nil {
		return "", fmt.Errorf("failed to serialize token: %w", err)
	}

	return tokenString, nil
}
