package api

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

// TestSandboxesTestSuite covers the Sandboxes API surface that doesn't depend
// on a live hydra/desktop-bridge being attached. CI doesn't run a sandbox host,
// so provisioning will end up in status=failed — that's fine. The point of
// this suite is to lock down:
//
//   - persistence flag round-trips correctly via the public REST API
//   - org-scoped isolation (cross-org id-guessing is rejected)
//   - listing/filtering by project
//   - delete is idempotent on already-failed sandboxes
//
// These are the parts most likely to regress when changing serializers,
// auth middleware, or the Sandbox/CreateSandboxRequest types.
func TestSandboxesTestSuite(t *testing.T) {
	suite.Run(t, new(SandboxesTestSuite))
}

type SandboxesTestSuite struct {
	suite.Suite
	ctx           context.Context
	db            *store.PostgresStore
	authenticator auth.Authenticator
}

func (s *SandboxesTestSuite) SetupTest() {
	if os.Getenv("START_HELIX_TEST_SERVER") != "true" {
		s.T().Skip("Skipping integration test - set START_HELIX_TEST_SERVER=true to enable")
	}
	s.ctx = context.Background()
	st, err := getStoreClient()
	s.Require().NoError(err)
	s.db = st

	cfg := &config.ServerConfig{}
	a, err := auth.NewHelixAuthenticator(cfg, s.db, "test-secret", nil)
	s.Require().NoError(err)
	s.authenticator = a
}

// userClient creates a fresh user and returns an authenticated API client.
func (s *SandboxesTestSuite) userClient(prefix string) (*client.HelixClient, string) {
	email := fmt.Sprintf("%s-%s@test.com", prefix, uuid.New().String())
	user, apiKey, err := createUser(s.T(), s.db, s.authenticator, email)
	s.Require().NoError(err)
	s.Require().NotNil(user)
	c, err := getAPIClient(apiKey)
	s.Require().NoError(err)
	return c, user.ID
}

func (s *SandboxesTestSuite) newOrg(c *client.HelixClient) *types.Organization {
	org, err := c.CreateOrganization(s.ctx, &types.Organization{
		Name: "sbx-test-" + uuid.New().String()[:8],
	})
	s.Require().NoError(err)
	s.Require().NotNil(org)
	return org
}

// TestPersistentFlagRoundTrips creates two sandboxes — one with persistent=true
// and one with persistent=false — and verifies the flag survives the
// create→get→list path without being silently dropped by a serializer.
func (s *SandboxesTestSuite) TestPersistentFlagRoundTrips() {
	c, _ := s.userClient("sandbox-persist")
	org := s.newOrg(c)

	persisted, err := c.CreateSandbox(s.ctx, org.ID, &types.CreateSandboxRequest{
		Name:           "persistent-fixture",
		Runtime:        "headless-ubuntu",
		TimeoutSeconds: 60,
		Persistent:     true,
	})
	s.Require().NoError(err)
	s.Require().NotNil(persisted)
	s.Require().Equal(true, persisted.Persistent, "Persistent must round-trip on create response")
	s.Require().Equal("headless-ubuntu", string(persisted.Runtime))

	ephemeral, err := c.CreateSandbox(s.ctx, org.ID, &types.CreateSandboxRequest{
		Name:           "ephemeral-fixture",
		Runtime:        "headless-ubuntu",
		TimeoutSeconds: 60,
		Persistent:     false,
	})
	s.Require().NoError(err)
	s.Require().Equal(false, ephemeral.Persistent)

	// Re-fetch via Get and assert the field survives the read path too.
	got, err := c.GetSandbox(s.ctx, org.ID, persisted.ID)
	s.Require().NoError(err)
	s.Require().Equal(true, got.Persistent, "Persistent must round-trip on Get")

	got, err = c.GetSandbox(s.ctx, org.ID, ephemeral.ID)
	s.Require().NoError(err)
	s.Require().Equal(false, got.Persistent)

	// And via List — making sure the listing serializer doesn't drop it either.
	resp, err := c.ListSandboxes(s.ctx, org.ID, nil)
	s.Require().NoError(err)
	byID := map[string]*types.Sandbox{}
	for _, sb := range resp.Sandboxes {
		byID[sb.ID] = sb
	}
	s.Require().Contains(byID, persisted.ID)
	s.Require().Equal(true, byID[persisted.ID].Persistent)
	s.Require().Contains(byID, ephemeral.ID)
	s.Require().Equal(false, byID[ephemeral.ID].Persistent)

	// Cleanup. Delete is idempotent against a sandbox whose container never
	// came up (no hydra host in CI), so we don't tolerate errors.
	s.Require().NoError(c.DeleteSandbox(s.ctx, org.ID, persisted.ID))
	s.Require().NoError(c.DeleteSandbox(s.ctx, org.ID, ephemeral.ID))
}

// TestRuntimesEndpointAdvertisesUbuntuDesktop guards the runtime registry
// initialisation. If the heartbeat-versioned ubuntu-desktop spec stops being
// registered (or HELIX_SANDBOX_RUNTIMES gets clobbered), this catches it.
func (s *SandboxesTestSuite) TestRuntimesEndpointAdvertisesUbuntuDesktop() {
	c, _ := s.userClient("sandbox-runtimes")
	runtimes, err := c.ListSandboxRuntimes(s.ctx)
	s.Require().NoError(err)
	s.Require().NotEmpty(runtimes)
	s.Require().Contains(runtimes, "ubuntu-desktop", "ubuntu-desktop runtime must always be registered (built-in)")
	s.Require().Contains(runtimes, "headless-ubuntu", "headless-ubuntu runtime must be registered by default")
}

// TestCrossOrgAccessRejected verifies that a user in org A cannot read or
// delete a sandbox owned by org B by guessing its id. This is the key
// authorization invariant for the Sandboxes API.
func (s *SandboxesTestSuite) TestCrossOrgAccessRejected() {
	owner, _ := s.userClient("sandbox-owner")
	intruder, _ := s.userClient("sandbox-intruder")

	ownerOrg := s.newOrg(owner)
	intruderOrg := s.newOrg(intruder)

	sb, err := owner.CreateSandbox(s.ctx, ownerOrg.ID, &types.CreateSandboxRequest{
		Name:           "private",
		Runtime:        "headless-ubuntu",
		TimeoutSeconds: 60,
		Persistent:     true,
	})
	s.Require().NoError(err)
	s.Require().NotNil(sb)

	// Intruder tries to fetch by guessing the id under their own org —
	// should 404, NOT leak that the id exists.
	_, err = intruder.GetSandbox(s.ctx, intruderOrg.ID, sb.ID)
	s.Require().Error(err, "GetSandbox across orgs must fail")

	// Intruder tries to delete via their own org context — same.
	err = intruder.DeleteSandbox(s.ctx, intruderOrg.ID, sb.ID)
	s.Require().Error(err, "DeleteSandbox across orgs must fail")

	// Sandbox should still exist for the owner.
	got, err := owner.GetSandbox(s.ctx, ownerOrg.ID, sb.ID)
	s.Require().NoError(err)
	s.Require().Equal(sb.ID, got.ID)
	s.Require().Equal(true, got.Persistent)

	s.Require().NoError(owner.DeleteSandbox(s.ctx, ownerOrg.ID, sb.ID))
}

// TestProjectFilter narrows the listing by project_id and checks that the
// optional project association is honoured by the persistence path.
func (s *SandboxesTestSuite) TestProjectFilter() {
	c, _ := s.userClient("sandbox-project")
	org := s.newOrg(c)

	// Two different "project" buckets — these are arbitrary string ids;
	// the Sandboxes API doesn't validate them against the projects table
	// (intentional: project association is informational here).
	const projectA = "prj_aaaaaaaaaaaaaaaaaaaaaa"
	const projectB = "prj_bbbbbbbbbbbbbbbbbbbbbb"

	a1, err := c.CreateSandbox(s.ctx, org.ID, &types.CreateSandboxRequest{
		Name: "a-1", Runtime: "headless-ubuntu", TimeoutSeconds: 60,
		ProjectID: projectA,
	})
	s.Require().NoError(err)
	a2, err := c.CreateSandbox(s.ctx, org.ID, &types.CreateSandboxRequest{
		Name: "a-2", Runtime: "headless-ubuntu", TimeoutSeconds: 60,
		ProjectID: projectA,
	})
	s.Require().NoError(err)
	b1, err := c.CreateSandbox(s.ctx, org.ID, &types.CreateSandboxRequest{
		Name: "b-1", Runtime: "headless-ubuntu", TimeoutSeconds: 60,
		ProjectID: projectB,
	})
	s.Require().NoError(err)

	listA, err := c.ListSandboxes(s.ctx, org.ID, &client.SandboxListFilter{ProjectID: projectA})
	s.Require().NoError(err)
	gotIDsA := idSet(listA.Sandboxes)
	s.Require().Contains(gotIDsA, a1.ID)
	s.Require().Contains(gotIDsA, a2.ID)
	s.Require().NotContains(gotIDsA, b1.ID, "filter must exclude other project")

	listB, err := c.ListSandboxes(s.ctx, org.ID, &client.SandboxListFilter{ProjectID: projectB})
	s.Require().NoError(err)
	gotIDsB := idSet(listB.Sandboxes)
	s.Require().Contains(gotIDsB, b1.ID)
	s.Require().NotContains(gotIDsB, a1.ID)
	s.Require().NotContains(gotIDsB, a2.ID)

	for _, id := range []string{a1.ID, a2.ID, b1.ID} {
		s.Require().NoError(c.DeleteSandbox(s.ctx, org.ID, id))
	}
}

// TestProvisioningEventuallyFailsWithoutHydra documents — and locks in — the
// CI-mode behaviour: with no hydra host registered, a sandbox transitions
// from pending → failed within a few seconds. This is a regression guard for
// the controller's failure path; it shouldn't hang indefinitely or panic.
func (s *SandboxesTestSuite) TestProvisioningEventuallyFailsWithoutHydra() {
	c, _ := s.userClient("sandbox-failpath")
	org := s.newOrg(c)

	sb, err := c.CreateSandbox(s.ctx, org.ID, &types.CreateSandboxRequest{
		Name:           "no-host-here",
		Runtime:        "headless-ubuntu",
		TimeoutSeconds: 60,
		Persistent:     true,
	})
	s.Require().NoError(err)
	s.Require().NotNil(sb)
	s.Require().Equal(types.SandboxStatusPending, sb.Status, "Create must respond with status=pending immediately")

	// Poll up to 10s for the controller's provision goroutine to give up.
	deadline := time.Now().Add(10 * time.Second)
	var last *types.Sandbox
	for time.Now().Before(deadline) {
		got, err := c.GetSandbox(s.ctx, org.ID, sb.ID)
		s.Require().NoError(err)
		last = got
		if got.Status == types.SandboxStatusFailed {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	s.Require().NotNil(last)
	s.Require().Equal(types.SandboxStatusFailed, last.Status,
		"without a hydra host registered the controller must transition to failed, not hang. status_message=%q", last.StatusMessage)
	s.Require().NotEmpty(last.StatusMessage, "failed sandbox should carry an explanation in status_message")

	// Persistent flag is still preserved on a failed sandbox.
	s.Require().Equal(true, last.Persistent)

	s.Require().NoError(c.DeleteSandbox(s.ctx, org.ID, sb.ID))
}

func idSet(items []*types.Sandbox) map[string]struct{} {
	out := map[string]struct{}{}
	for _, it := range items {
		out[it.ID] = struct{}{}
	}
	return out
}
