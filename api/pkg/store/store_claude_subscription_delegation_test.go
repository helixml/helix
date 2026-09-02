package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/system"
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

func (suite *PostgresStoreTestSuite) TestUpdateClaudeSubscriptionDelegationConcurrent() {
	suffix := system.GenerateUUID()
	org, err := suite.db.CreateOrganization(suite.ctx, &types.Organization{
		ID: system.GenerateOrganizationID(), Name: "delegation-race-" + suffix, Owner: "usr_owner_" + suffix,
	})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _ = suite.db.DeleteOrganization(context.Background(), org.ID) })
	create := func(owner string) *types.ClaudeSubscription {
		sub, err := suite.db.CreateClaudeSubscription(suite.ctx, &types.ClaudeSubscription{
			OwnerID: owner, OwnerType: types.OwnerTypeUser, Status: "active",
			EncryptedCredentials: "encrypted", CreatedBy: owner,
		})
		suite.Require().NoError(err)
		suite.T().Cleanup(func() { _ = suite.db.DeleteClaudeSubscription(context.Background(), sub.ID) })
		return sub
	}
	subA := create("usr_a_" + suffix)
	subB := create("usr_b_" + suffix)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, sub := range []*types.ClaudeSubscription{subA, subB} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-start
			_, err := suite.db.UpdateClaudeSubscriptionDelegation(context.Background(), id, []string{org.ID})
			results <- err
		}(sub.ID)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for result := range results {
		if result == nil {
			successes++
			continue
		}
		var conflict *ClaudeSubscriptionDelegationConflictError
		suite.Require().True(errors.As(result, &conflict), result.Error())
		suite.Equal(org.ID, conflict.OrganizationID)
		conflicts++
	}
	suite.Equal(1, successes)
	suite.Equal(1, conflicts)

	delegated, err := suite.db.GetDelegatedClaudeSubscriptionForOrg(suite.ctx, org.ID)
	suite.Require().NoError(err)
	suite.Contains([]string{subA.ID, subB.ID}, delegated.ID)
}

func (suite *PostgresStoreTestSuite) TestDelegatedClaudeSubscriptionResolution() {
	suffix := system.GenerateUUID()
	orgID := "org_" + suffix
	create := func(owner string, orgs ...string) *types.ClaudeSubscription {
		sub, err := suite.db.CreateClaudeSubscription(suite.ctx, &types.ClaudeSubscription{
			OwnerID: owner, OwnerType: types.OwnerTypeUser, Status: "active",
			EncryptedCredentials: "encrypted", CreatedBy: owner, DelegatedOrgIDs: orgs,
		})
		suite.Require().NoError(err)
		suite.T().Cleanup(func() { _ = suite.db.DeleteClaudeSubscription(context.Background(), sub.ID) })
		return sub
	}

	delegated := create("usr_delegate_"+suffix, orgID)
	owner := create("usr_owner_" + suffix)

	sub, err := suite.db.GetDelegatedClaudeSubscriptionForOrg(suite.ctx, orgID)
	suite.Require().NoError(err)
	suite.Equal(delegated.ID, sub.ID)

	sub, err = suite.db.GetSessionClaudeSubscription(suite.ctx, &types.Session{
		Owner: owner.OwnerID, OrganizationID: orgID,
		Metadata: types.SessionMetadata{SpecTaskID: "spt_1"},
	})
	suite.Require().NoError(err)
	suite.Equal(delegated.ID, sub.ID)

	sub, err = suite.db.GetSessionClaudeSubscription(suite.ctx, &types.Session{
		Owner: owner.OwnerID, OrganizationID: orgID,
	})
	suite.Require().NoError(err)
	suite.Equal(owner.ID, sub.ID)

	sub, err = suite.db.GetSessionClaudeSubscription(suite.ctx, &types.Session{
		Owner: owner.OwnerID, OrganizationID: orgID,
		Metadata: types.SessionMetadata{SpecTaskID: "spt_1", CredentialOwnerID: delegated.OwnerID},
	})
	suite.Require().NoError(err)
	suite.Equal(delegated.ID, sub.ID)

	undelegated := create("usr_undelegated_" + suffix)
	_, err = suite.db.GetSessionClaudeSubscription(suite.ctx, &types.Session{
		Owner: owner.OwnerID, OrganizationID: orgID,
		Metadata: types.SessionMetadata{SpecTaskID: "spt_1", CredentialOwnerID: undelegated.OwnerID},
	})
	suite.True(errors.Is(err, ErrNotFound))

	create("usr_other_"+suffix, orgID)
	_, err = suite.db.GetDelegatedClaudeSubscriptionForOrg(suite.ctx, orgID)
	suite.ErrorContains(err, "multiple Claude subscriptions")
	_, err = suite.db.GetDelegatedClaudeSubscriptionForOrg(suite.ctx, "org_missing_"+suffix)
	suite.True(errors.Is(err, ErrNotFound))
}
