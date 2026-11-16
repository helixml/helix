package server

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/orgstore"
	"github.com/helixml/helix/api/pkg/types"
)

// An org API key acts as its owner only inside its organization. The owner
// shortcuts ("the caller created it") must not carry it to the owner's
// resources in another organization or outside every organization.

func orgKey(orgID string) *types.User {
	return &types.User{ID: "usr_owner", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, OrganizationID: orgID}
}

func personalKey() *types.User {
	return &types.User{ID: "usr_owner", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI}
}

func TestCallerOwns_OrgKeyConfinedToItsOrg(t *testing.T) {
	require.True(t, callerOwns(personalKey(), "usr_owner", ""))
	require.True(t, callerOwns(personalKey(), "usr_owner", "org_a"))
	require.True(t, callerOwns(orgKey("org_a"), "usr_owner", "org_a"))
	require.False(t, callerOwns(orgKey("org_b"), "usr_owner", "org_a"), "org B key reached org A")
	require.False(t, callerOwns(orgKey("org_b"), "usr_owner", ""), "org key reached a personal resource")
	require.False(t, callerOwns(personalKey(), "usr_other", ""))
	require.False(t, callerOwns(&types.User{}, "", ""))
}

func TestCanMoveProject_OnlyOwnersUnscopedCredential(t *testing.T) {
	personal := &types.Project{ID: "prj_personal", UserID: "usr_owner"}
	require.True(t, canMoveProject(personalKey(), personal))
	require.False(t, canMoveProject(orgKey("org_a"), personal), "org key moved its owner's personal project")
	require.False(t, canMoveProject(ownerProjectKey(), personal), "project key moved a project outside its scope")
	require.False(t, canMoveProject(&types.User{ID: "usr_other"}, personal))
}

func TestOwnsKnowledge_OrgKeyUsesTheAgentsOrg(t *testing.T) {
	ctx := context.Background()
	srv, st := newScopedKeyTestServer(t)
	st.EXPECT().GetApp(gomock.Any(), "app_a").Return(&types.App{ID: "app_a", OrganizationID: "org_a", Owner: "usr_owner"}, nil).AnyTimes()

	// Knowledge rows created with an agent carry no organization of their
	// own; the agent's decides.
	knowledge := &types.Knowledge{ID: "kno_1", Owner: "usr_owner", AppID: "app_a"}
	require.True(t, srv.ownsKnowledge(ctx, orgKey("org_a"), knowledge))
	require.False(t, srv.ownsKnowledge(ctx, orgKey("org_b"), knowledge), "org B key reached org A's knowledge")
	require.True(t, srv.ownsKnowledge(ctx, personalKey(), knowledge))
	require.False(t, srv.ownsKnowledge(ctx, orgKey("org_a"), &types.Knowledge{ID: "kno_2", Owner: "usr_owner"}),
		"org key reached knowledge with no agent")
}

func TestAuthorizeUserToTrigger_OrgKeyConfinedToTheAgentsOrg(t *testing.T) {
	ctx := context.Background()
	srv, st := newScopedKeyTestServer(t)
	st.EXPECT().GetApp(gomock.Any(), "app_a").Return(&types.App{ID: "app_a", OrganizationID: "org_a", Owner: "usr_owner"}, nil).AnyTimes()
	trigger := &types.TriggerConfiguration{ID: "trg_1", Owner: "usr_owner", AppID: "app_a"}

	err := srv.authorizeUserToTrigger(ctx, orgKey("org_b"), trigger)
	require.True(t, errors.Is(err, orgstore.ErrKeyOrgScope), "org B key reached org A's trigger: %v", err)
	require.NoError(t, srv.authorizeUserToTrigger(ctx, orgKey("org_a"), trigger))
	require.NoError(t, srv.authorizeUserToTrigger(ctx, personalKey(), trigger))
}

func TestAuthorizeUserToRepositoryAccessGrants_OrgKeyOfAnotherOrg(t *testing.T) {
	srv, _ := newScopedKeyTestServer(t)
	repo := &types.GitRepository{ID: "repo_a", OwnerID: "usr_owner", OrganizationID: "org_a"}
	require.Error(t, srv.authorizeUserToRepositoryAccessGrants(context.Background(), orgKey("org_b"), repo))
}

func TestOwnsSessionInKeyScope(t *testing.T) {
	orgSession := &types.Session{ID: "ses_b", Owner: "usr_owner", OrganizationID: "org_a", ProjectID: "prj_b"}
	require.True(t, ownsSessionInKeyScope(personalKey(), orgSession))
	require.True(t, ownsSessionInKeyScope(orgKey("org_a"), orgSession))
	require.False(t, ownsSessionInKeyScope(orgKey("org_b"), orgSession), "org B key reached org A's session")
	require.False(t, ownsSessionInKeyScope(ownerProjectKey(), orgSession), "project key reached another project's session")
}

func TestListSecrets_OrgKeyHasNoPersonalSecrets(t *testing.T) {
	srv, _ := newScopedKeyTestServer(t)
	_, err := srv.listSecrets(nil, requestAs(http.MethodGet, "/api/v1/secrets", "", orgKey("org_a"), nil))
	requireHTTPStatus(t, err, http.StatusForbidden)
}
