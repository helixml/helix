package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestHelixAuthenticatorTestSuite(t *testing.T) {
	suite.Run(t, new(HelixAuthenticatorTestSuite))
}

type HelixAuthenticatorTestSuite struct {
	suite.Suite
	ctx      context.Context
	store    *store.MockStore
	notifier *notification.MockNotifier
	auth     *HelixAuthenticator
	cfg      *config.ServerConfig
}

func (suite *HelixAuthenticatorTestSuite) SetupTest() {
	suite.ctx = context.Background()
	ctrl := gomock.NewController(suite.T())
	suite.store = store.NewMockStore(ctrl)
	suite.notifier = notification.NewMockNotifier(ctrl)

	suite.cfg = &config.ServerConfig{
		WebServer: config.WebServer{
			URL: "http://localhost:3000",
		},
	}

	var err error
	suite.auth, err = NewHelixAuthenticator(suite.cfg, suite.store, "test-secret-key", suite.notifier)
	suite.Require().NoError(err)
	suite.Require().NotNil(suite.auth)
}

func (suite *HelixAuthenticatorTestSuite) TestGetUserByID() {
	userID := uuid.New().String()
	expectedUser := &types.User{
		ID:       userID,
		Email:    "test@example.com",
		Username: "testuser",
		FullName: "Test User",
	}

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).
		Return(expectedUser, nil)

	user, err := suite.auth.GetUserByID(suite.ctx, userID)
	suite.NoError(err)
	suite.NotNil(user)
	suite.Equal(userID, user.ID)
	suite.Equal(expectedUser.Email, user.Email)
}

func (suite *HelixAuthenticatorTestSuite) TestGetUserByID_NotFound() {
	userID := uuid.New().String()

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).
		Return(nil, errors.New("user not found"))

	user, err := suite.auth.GetUserByID(suite.ctx, userID)
	suite.Error(err)
	suite.Nil(user)
}

func (suite *HelixAuthenticatorTestSuite) TestCreateUser() {
	userID := uuid.New().String()
	password := "testpassword123"
	user := &types.User{
		ID:       userID,
		Email:    "newuser@example.com",
		Username: "newuser",
		Password: password,
	}

	suite.store.EXPECT().
		CreateUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
			suite.NotEmpty(u.PasswordHash)
			suite.Empty(u.Password)
			suite.Equal(userID, u.ID)
			suite.Equal(user.Email, u.Email)
			return u, nil
		})

	createdUser, err := suite.auth.CreateUser(suite.ctx, user)
	suite.NoError(err)
	suite.NotNil(createdUser)
	suite.Equal(userID, createdUser.ID)
	suite.Empty(createdUser.Password)
}

func (suite *HelixAuthenticatorTestSuite) TestCreateUser_WithoutPassword() {
	userID := uuid.New().String()
	user := &types.User{
		ID:       userID,
		Email:    "nopass@example.com",
		Username: "nopass",
		Password: "",
	}

	suite.store.EXPECT().
		CreateUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
			suite.Empty(u.PasswordHash)
			suite.Empty(u.Password)
			return u, nil
		})

	createdUser, err := suite.auth.CreateUser(suite.ctx, user)
	suite.NoError(err)
	suite.NotNil(createdUser)
}

func (suite *HelixAuthenticatorTestSuite) TestCreateUser_DevMode_SetsAdminTrue() {
	// Set ADMIN_USERS=all (dev mode)
	suite.cfg.WebServer.AdminUsers = config.AdminAllUsers

	userID := uuid.New().String()
	user := &types.User{
		ID:       userID,
		Email:    "devuser@example.com",
		Username: "devuser",
		Password: "testpassword123",
		Admin:    false, // Initially not admin
	}

	suite.store.EXPECT().
		CreateUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
			// In dev mode, user should be created with Admin=true
			suite.True(u.Admin, "user should be created as admin in dev mode")
			return u, nil
		})

	createdUser, err := suite.auth.CreateUser(suite.ctx, user)
	suite.NoError(err)
	suite.NotNil(createdUser)
}

func (suite *HelixAuthenticatorTestSuite) TestCreateUser_ProductionMode_PreservesAdminFalse() {
	// Ensure we're in production mode (AdminUsers is empty)
	suite.cfg.WebServer.AdminUsers = ""

	userID := uuid.New().String()
	user := &types.User{
		ID:       userID,
		Email:    "produser@example.com",
		Username: "produser",
		Password: "testpassword123",
		Admin:    false,
	}

	suite.store.EXPECT().
		CreateUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
			// In production mode, user should NOT be automatically made admin
			suite.False(u.Admin, "user should not be admin in production mode")
			return u, nil
		})

	createdUser, err := suite.auth.CreateUser(suite.ctx, user)
	suite.NoError(err)
	suite.NotNil(createdUser)
}

func (suite *HelixAuthenticatorTestSuite) TestUpdatePassword() {
	userID := uuid.New().String()
	newPassword := "newpassword123"
	existingUser := &types.User{
		ID:           userID,
		Email:        "test@example.com",
		PasswordHash: []byte("old-hash"),
	}

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).
		Return(existingUser, nil)

	suite.store.EXPECT().
		UpdateUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, u *types.User) (*types.User, error) {
			suite.NotEmpty(u.PasswordHash)
			suite.Empty(u.Password)
			err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(newPassword))
			suite.NoError(err)
			return u, nil
		})

	err := suite.auth.UpdatePassword(suite.ctx, userID, newPassword)
	suite.NoError(err)
}

func (suite *HelixAuthenticatorTestSuite) TestUpdatePassword_TooShort() {
	userID := uuid.New().String()
	shortPassword := "short"

	err := suite.auth.UpdatePassword(suite.ctx, userID, shortPassword)
	suite.Error(err)
	suite.Contains(err.Error(), "password must be between")
}

func (suite *HelixAuthenticatorTestSuite) TestUpdatePassword_TooLong() {
	userID := uuid.New().String()
	longPassword := string(make([]byte, 129))

	err := suite.auth.UpdatePassword(suite.ctx, userID, longPassword)
	suite.Error(err)
	suite.Contains(err.Error(), "password must be between")
}

func (suite *HelixAuthenticatorTestSuite) TestUpdatePassword_UserNotFound() {
	userID := uuid.New().String()
	newPassword := "newpassword123"

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).
		Return(nil, errors.New("user not found"))

	err := suite.auth.UpdatePassword(suite.ctx, userID, newPassword)
	suite.Error(err)
	suite.Contains(err.Error(), "failed to get user")
}

func (suite *HelixAuthenticatorTestSuite) TestValidatePassword() {
	password := "testpassword123"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	suite.Require().NoError(err)

	user := &types.User{
		ID:           uuid.New().String(),
		PasswordHash: hash,
	}

	err = suite.auth.ValidatePassword(suite.ctx, user, password)
	suite.NoError(err)
}

func (suite *HelixAuthenticatorTestSuite) TestValidatePassword_Invalid() {
	password := "testpassword123"
	wrongPassword := "wrongpassword"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	suite.Require().NoError(err)

	user := &types.User{
		ID:           uuid.New().String(),
		PasswordHash: hash,
	}

	err = suite.auth.ValidatePassword(suite.ctx, user, wrongPassword)
	suite.Error(err)
}

func (suite *HelixAuthenticatorTestSuite) TestGenerateUserToken() {
	user := &types.User{
		ID:    uuid.New().String(),
		Email: "test@example.com",
		Admin: false,
	}

	token, err := suite.auth.GenerateUserToken(suite.ctx, user)
	suite.NoError(err)
	suite.NotEmpty(token)
}

func (suite *HelixAuthenticatorTestSuite) TestGenerateUserToken_NilUser() {
	token, err := suite.auth.GenerateUserToken(suite.ctx, nil)
	suite.Error(err)
	suite.Empty(token)
	suite.Contains(err.Error(), "user is required")
}

func (suite *HelixAuthenticatorTestSuite) TestValidateUserToken() {
	user := &types.User{
		ID:    uuid.New().String(),
		Email: "test@example.com",
		Admin: false,
	}

	token, err := suite.auth.GenerateUserToken(suite.ctx, user)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(token)

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).
		Return(user, nil)

	validatedUser, err := suite.auth.ValidateUserToken(suite.ctx, token)
	suite.NoError(err)
	suite.NotNil(validatedUser)
	suite.Equal(user.ID, validatedUser.ID)
	suite.Equal(user.Email, validatedUser.Email)
	suite.Equal(token, validatedUser.Token)
	suite.Equal(user.Admin, validatedUser.Admin)
}

func (suite *HelixAuthenticatorTestSuite) TestValidateUserToken_Admin() {
	user := &types.User{
		ID:    uuid.New().String(),
		Email: "test@example.com",
		Admin: false,
	}

	// Set ADMIN_USERS=all (dev mode) where everyone is admin
	suite.cfg.WebServer.AdminUsers = config.AdminAllUsers

	token, err := suite.auth.GenerateUserToken(suite.ctx, user)
	suite.Require().NoError(err)
	suite.Require().NotEmpty(token)

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).
		Return(user, nil)

	validatedUser, err := suite.auth.ValidateUserToken(suite.ctx, token)
	suite.NoError(err)
	suite.NotNil(validatedUser)
	suite.Equal(user.ID, validatedUser.ID)
	suite.Equal(user.Email, validatedUser.Email)
	suite.Equal(token, validatedUser.Token)
	suite.True(validatedUser.Admin)
}

func (suite *HelixAuthenticatorTestSuite) TestValidateUserToken_EmptyToken() {
	user, err := suite.auth.ValidateUserToken(suite.ctx, "")
	suite.Error(err)
	suite.Nil(user)
	suite.Contains(err.Error(), "access token is required")
}

func (suite *HelixAuthenticatorTestSuite) TestValidateUserToken_InvalidToken() {
	invalidToken := "invalid.token.here"

	user, err := suite.auth.ValidateUserToken(suite.ctx, invalidToken)
	suite.Error(err)
	suite.Nil(user)
}

func (suite *HelixAuthenticatorTestSuite) TestValidateUserToken_ExpiredToken() {
	user := &types.User{
		ID:    uuid.New().String(),
		Email: "test@example.com",
	}

	authWithShortExpiry, err := NewHelixAuthenticator(suite.cfg, suite.store, "test-secret-key", suite.notifier)
	suite.Require().NoError(err)

	token, err := authWithShortExpiry.GenerateUserToken(suite.ctx, user)
	suite.Require().NoError(err)

	time.Sleep(100 * time.Millisecond)

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).
		Return(user, nil)

	validatedUser, err := suite.auth.ValidateUserToken(suite.ctx, token)
	suite.NoError(err)
	suite.NotNil(validatedUser)
}

func (suite *HelixAuthenticatorTestSuite) TestValidateUserToken_UserNotFound() {
	user := &types.User{
		ID:    uuid.New().String(),
		Email: "test@example.com",
	}

	token, err := suite.auth.GenerateUserToken(suite.ctx, user)
	suite.Require().NoError(err)

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).
		Return(nil, errors.New("user not found"))

	validatedUser, err := suite.auth.ValidateUserToken(suite.ctx, token)
	suite.Error(err)
	suite.Nil(validatedUser)
	suite.Contains(err.Error(), "failed to get user")
}

func (suite *HelixAuthenticatorTestSuite) TestRequestPasswordReset() {
	userID := uuid.New().String()
	email := "test@example.com"
	user := &types.User{
		ID:    userID,
		Email: email,
	}

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{Email: email}).
		Return(user, nil)

	suite.notifier.EXPECT().
		Notify(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, n *types.Notification) error {
			suite.Equal(email, n.Email)
			suite.Equal(types.EventPasswordResetRequest, n.Event)
			suite.Contains(n.Message, "password-reset-complete")
			suite.True(n.RenderMarkdown)
			return nil
		})

	err := suite.auth.RequestPasswordReset(suite.ctx, email)
	suite.NoError(err)
}

func (suite *HelixAuthenticatorTestSuite) TestRequestPasswordReset_UserNotFound() {
	email := "nonexistent@example.com"

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{Email: email}).
		Return(nil, errors.New("user not found"))

	err := suite.auth.RequestPasswordReset(suite.ctx, email)
	suite.Error(err)
	suite.Contains(err.Error(), "failed to get user")
}

func (suite *HelixAuthenticatorTestSuite) TestRequestPasswordReset_NilUser() {
	email := "nonexistent@example.com"

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{Email: email}).
		Return(nil, nil)

	err := suite.auth.RequestPasswordReset(suite.ctx, email)
	suite.Error(err)
	suite.Contains(err.Error(), "user not found")
}

func (suite *HelixAuthenticatorTestSuite) TestRequestPasswordReset_NotificationError() {
	userID := uuid.New().String()
	email := "test@example.com"
	user := &types.User{
		ID:    userID,
		Email: email,
	}

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{Email: email}).
		Return(user, nil)

	suite.notifier.EXPECT().
		Notify(gomock.Any(), gomock.Any()).
		Return(errors.New("notification service unavailable"))

	err := suite.auth.RequestPasswordReset(suite.ctx, email)
	suite.Error(err)
	suite.Contains(err.Error(), "failed to send password reset email")
}

func (suite *HelixAuthenticatorTestSuite) TestPasswordResetComplete() {
	userID := uuid.New().String()
	newPassword := "newpassword123"
	user := &types.User{
		ID:           userID,
		Email:        "test@example.com",
		PasswordHash: []byte("old-hash"),
	}

	token, err := suite.auth.GenerateUserToken(suite.ctx, user)
	suite.Require().NoError(err)

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).
		Return(user, nil)

	suite.store.EXPECT().
		UpdateUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, u *types.User) (*types.User, error) {
			suite.NotEmpty(u.PasswordHash)
			suite.Empty(u.Password)
			err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(newPassword))
			suite.NoError(err)
			return u, nil
		})

	err = suite.auth.PasswordResetComplete(suite.ctx, token, newPassword)
	suite.NoError(err)
}

func (suite *HelixAuthenticatorTestSuite) TestPasswordResetComplete_InvalidToken() {
	newPassword := "newpassword123"
	invalidToken := "invalid.token.here"

	err := suite.auth.PasswordResetComplete(suite.ctx, invalidToken, newPassword)
	suite.Error(err)
	suite.Contains(err.Error(), "failed to parse token")
}

func (suite *HelixAuthenticatorTestSuite) TestPasswordResetComplete_TooShortPassword() {
	userID := uuid.New().String()
	user := &types.User{
		ID:    userID,
		Email: "test@example.com",
	}

	token, err := suite.auth.GenerateUserToken(suite.ctx, user)
	suite.Require().NoError(err)

	shortPassword := "short"

	err = suite.auth.PasswordResetComplete(suite.ctx, token, shortPassword)
	suite.Error(err)
	suite.Contains(err.Error(), "password must be between")
}

func (suite *HelixAuthenticatorTestSuite) TestPasswordResetComplete_TooLongPassword() {
	userID := uuid.New().String()
	user := &types.User{
		ID:    userID,
		Email: "test@example.com",
	}

	token, err := suite.auth.GenerateUserToken(suite.ctx, user)
	suite.Require().NoError(err)

	longPassword := string(make([]byte, 129))

	err = suite.auth.PasswordResetComplete(suite.ctx, token, longPassword)
	suite.Error(err)
	suite.Contains(err.Error(), "password must be between")
}

func (suite *HelixAuthenticatorTestSuite) TestPasswordResetComplete_UserNotFound() {
	userID := uuid.New().String()
	newPassword := "newpassword123"
	user := &types.User{
		ID:    userID,
		Email: "test@example.com",
	}

	token, err := suite.auth.GenerateUserToken(suite.ctx, user)
	suite.Require().NoError(err)

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).
		Return(nil, errors.New("user not found"))

	err = suite.auth.PasswordResetComplete(suite.ctx, token, newPassword)
	suite.Error(err)
	suite.Contains(err.Error(), "failed to get user")
}

func (suite *HelixAuthenticatorTestSuite) TestNewHelixAuthenticator_DefaultSecret() {
	auth, err := NewHelixAuthenticator(suite.cfg, suite.store, "", suite.notifier)
	suite.NoError(err)
	suite.NotNil(auth)
}

func (suite *HelixAuthenticatorTestSuite) TestGenerateUserToken_AdminFlag() {
	adminUser := &types.User{
		ID:    uuid.New().String(),
		Email: "admin@example.com",
		Admin: true,
	}

	token, err := suite.auth.GenerateUserToken(suite.ctx, adminUser)
	suite.NoError(err)
	suite.NotEmpty(token)

	suite.store.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: adminUser.ID}).
		Return(adminUser, nil)

	validatedUser, err := suite.auth.ValidateUserToken(suite.ctx, token)
	suite.NoError(err)
	suite.NotNil(validatedUser)
	suite.True(validatedUser.Admin)
}
