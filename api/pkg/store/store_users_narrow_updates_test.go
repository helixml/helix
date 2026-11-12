package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// The narrow updates touch only their own columns: a password changed after
// the row was read is not written back by a sign-in that read the old hash.
func TestUpdateUserEmailVerification_DoesNotClobberOtherColumns(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	id := system.GenerateUserID()
	created, err := db.CreateUser(ctx, &types.User{ID: id, Email: id + "@example.com", Username: id, PasswordHash: []byte("hash-v1"), Waitlisted: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.DeleteUser(ctx, id) })

	// A concurrent password change lands after the sign-in read its row.
	created.PasswordHash = []byte("hash-v2")
	_, err = db.UpdateUser(ctx, created)
	require.NoError(t, err)

	require.NoError(t, db.UpdateUserEmailVerification(ctx, id, true, "idp-subject-"+id))
	require.NoError(t, db.MarkUserOnboarded(ctx, id))

	u, err := db.GetUser(ctx, &GetUserQuery{ID: id})
	require.NoError(t, err)
	require.Equal(t, []byte("hash-v2"), u.PasswordHash, "the newer password hash survives")
	require.True(t, u.EmailVerified)
	require.Equal(t, "idp-subject-"+id, u.OIDCSubject)
	require.True(t, u.OnboardingCompleted)
	require.False(t, u.OnboardingCompletedAt.IsZero())
	require.False(t, u.Waitlisted)

	// Without a subject the existing link is kept.
	require.NoError(t, db.UpdateUserEmailVerification(ctx, id, true, ""))
	u, err = db.GetUser(ctx, &GetUserQuery{ID: id})
	require.NoError(t, err)
	require.Equal(t, "idp-subject-"+id, u.OIDCSubject)

	require.ErrorIs(t, db.UpdateUserEmailVerification(ctx, "usr_missing", true, ""), ErrNotFound)
	require.ErrorIs(t, db.MarkUserOnboarded(ctx, "usr_missing"), ErrNotFound)
}
