package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// The one-time backfill marks accounts created from an identity provider
// assertion as verified (SSO auth provider, or an ID that is the provider's
// subject rather than a usr_ password sign-up) and leaves password sign-ups
// alone. Running it again changes nothing.
func TestBackfillEmailVerified(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	tag := system.GenerateAppID()

	create := func(id string, provider types.AuthProvider) *types.User {
		u, err := db.CreateUser(ctx, &types.User{ID: id, Email: id + "@example.com", Username: id, AuthProvider: provider})
		require.NoError(t, err)
		return u
	}
	sso := create("idp-subject-"+tag, types.AuthProviderOIDC)
	ssoNoProvider := create("keycloak-subject-"+tag, "")
	password := create(system.GenerateUserID(), types.AuthProviderRegular)
	passwordNoProvider := create(system.GenerateUserID(), "")
	service := create("svc-"+tag, types.AuthProviderService)
	t.Cleanup(func() {
		for _, u := range []*types.User{sso, ssoNoProvider, password, passwordNoProvider, service} {
			_ = db.DeleteUser(ctx, u.ID)
		}
	})

	for i := 0; i < 2; i++ {
		require.NoError(t, db.backfillEmailVerified(ctx))

		verified := func(id string) bool {
			u, err := db.GetUser(ctx, &GetUserQuery{ID: id})
			require.NoError(t, err)
			return u.EmailVerified
		}
		require.True(t, verified(sso.ID), "SSO account")
		require.True(t, verified(ssoNoProvider.ID), "account whose ID is an IdP subject")
		require.False(t, verified(password.ID), "password sign-up")
		require.False(t, verified(passwordNoProvider.ID), "password sign-up without a recorded provider")
		require.False(t, verified(service.ID), "service account")
	}
}

// GetUser resolves an account by the identity provider subject it is linked
// to, independently of its ID.
func TestGetUser_ByOIDCSubject(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	id := system.GenerateUserID()
	subject := "idp-subject-" + system.GenerateAppID()
	_, err := db.CreateUser(ctx, &types.User{ID: id, Email: id + "@example.com", Username: id, OIDCSubject: subject})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DeleteUser(ctx, id) })

	u, err := db.GetUser(ctx, &GetUserQuery{OIDCSubject: subject})
	require.NoError(t, err)
	require.Equal(t, id, u.ID)

	_, err = db.GetUser(ctx, &GetUserQuery{OIDCSubject: "nobody-" + subject})
	require.ErrorIs(t, err, ErrNotFound)
}
