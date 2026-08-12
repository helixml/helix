package store

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

// subscriptionDelegatedTo is the consent gate for SpecTask.CredentialOwnerID.
// If it ever returns true without an explicit grant, any member who can create a
// task could spend another user's Claude quota — so pin the behaviour.
func TestSubscriptionDelegatedTo(t *testing.T) {
	tests := []struct {
		name  string
		sub   *types.ClaudeSubscription
		orgID string
		want  bool
	}{
		{
			name:  "granted for this org",
			sub:   &types.ClaudeSubscription{DelegatedOrgIDs: []string{"org_a", "org_b"}},
			orgID: "org_b",
			want:  true,
		},
		{
			name:  "granted for a different org only",
			sub:   &types.ClaudeSubscription{DelegatedOrgIDs: []string{"org_a"}},
			orgID: "org_b",
			want:  false,
		},
		{
			name:  "no grant at all is the default and must not delegate",
			sub:   &types.ClaudeSubscription{},
			orgID: "org_a",
			want:  false,
		},
		{
			name:  "empty org id never matches",
			sub:   &types.ClaudeSubscription{DelegatedOrgIDs: []string{""}},
			orgID: "",
			want:  false,
		},
		{
			name:  "nil subscription",
			sub:   nil,
			orgID: "org_a",
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := subscriptionDelegatedTo(tc.sub, tc.orgID); got != tc.want {
				t.Fatalf("subscriptionDelegatedTo() = %v, want %v", got, tc.want)
			}
		})
	}
}
