package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// OAuth rows have uuid primary keys; Postgres rejects any other string with
// SQLSTATE 22P02. A malformed id is answered as not found without querying
// (a nil database would panic if it were reached).
func TestOAuthLookups_MalformedIDIsNotFound(t *testing.T) {
	st := &PostgresStore{}
	ctx := context.Background()

	_, err := st.GetOAuthConnection(ctx, "HXG-01-marker")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = st.GetOAuthProvider(ctx, "github")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = st.GetOAuthConnectionByUserAndProvider(ctx, "usr_1", "not-a-uuid")
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, st.DeleteOAuthConnection(ctx, "not-a-uuid"))
	require.NoError(t, st.DeleteOAuthProvider(ctx, "not-a-uuid"))
}
