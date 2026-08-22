package server

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

func TestClaudeSubscriptionNeedsRefresh(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		sub  *types.ClaudeSubscription
		want bool
	}{
		{
			name: "nil is skipped",
			sub:  nil,
			want: false,
		},
		{
			// Setup tokens carry no refresh token and last about a year.
			name: "setup token is never refreshed",
			sub: &types.ClaudeSubscription{
				CredentialType:       "setup_token",
				AccessTokenExpiresAt: now.Add(-24 * time.Hour),
			},
			want: false,
		},
		{
			// Refreshing blind would burn a request on every sweep, forever.
			name: "unknown expiry is skipped",
			sub:  &types.ClaudeSubscription{CredentialType: "oauth"},
			want: false,
		},
		{
			name: "healthy token is left alone",
			sub: &types.ClaudeSubscription{
				CredentialType:       "oauth",
				AccessTokenExpiresAt: now.Add(6 * time.Hour),
			},
			want: false,
		},
		{
			name: "token inside the lead time is refreshed",
			sub: &types.ClaudeSubscription{
				CredentialType:       "oauth",
				AccessTokenExpiresAt: now.Add(30 * time.Minute),
			},
			want: true,
		},
		{
			// The case that used to strand a subscription: the access token is
			// dead but the refresh token still has days on it.
			name: "already expired token is still worth refreshing",
			sub: &types.ClaudeSubscription{
				CredentialType:       "oauth",
				AccessTokenExpiresAt: now.Add(-11 * time.Hour),
			},
			want: true,
		},
		{
			// Empty credential type means oauth, the pre-setup-token default.
			name: "empty credential type is treated as oauth",
			sub: &types.ClaudeSubscription{
				AccessTokenExpiresAt: now.Add(10 * time.Minute),
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := claudeSubscriptionNeedsRefresh(tc.sub, now); got != tc.want {
				t.Fatalf("claudeSubscriptionNeedsRefresh() = %v, want %v", got, tc.want)
			}
		})
	}
}

// The lead time must exceed the sweep interval, or a token can expire in the
// gap between two passes without ever being picked up.
func TestClaudeRefreshLeadTimeExceedsInterval(t *testing.T) {
	if claudeRefreshLeadTime <= claudeRefreshInterval {
		t.Fatalf("lead time %s must be longer than the sweep interval %s", claudeRefreshLeadTime, claudeRefreshInterval)
	}
}
