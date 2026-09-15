package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIsUserOnline(t *testing.T) {
	now := time.Now()
	recent := now.Add(-PresenceOnlineWindow + time.Second)
	stale := now.Add(-PresenceOnlineWindow)

	require.False(t, IsUserOnline(nil, now))
	require.False(t, IsUserOnline(&User{}, now))
	require.True(t, IsUserOnline(&User{LastSeenAt: &recent}, now))
	require.False(t, IsUserOnline(&User{LastSeenAt: &stale}, now))
}
