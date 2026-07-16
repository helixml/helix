package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/crypto"
	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	goslack "github.com/slack-go/slack"
)

type slackWorkspaceTestStore struct {
	helixstore.Store
	connections []*types.ServiceConnection
	updated     *types.ServiceConnection
}

func (s *slackWorkspaceTestStore) ListServiceConnectionsByType(_ context.Context, _ string, _ types.ServiceConnectionType) ([]*types.ServiceConnection, error) {
	return s.connections, nil
}

func (s *slackWorkspaceTestStore) UpdateServiceConnection(_ context.Context, conn *types.ServiceConnection) error {
	s.updated = conn
	return nil
}

func TestSelectSlackWorkspaceRejectsAmbiguousDefault(t *testing.T) {
	conns := []*types.ServiceConnection{
		{ID: "conn-1", SlackTeamID: "T1"},
		{ID: "conn-2", SlackTeamID: "T2"},
	}
	if _, err := selectSlackWorkspace(conns, ""); !errors.Is(err, slacktransport.ErrAmbiguousWorkspace) {
		t.Fatalf("error = %v, want ErrAmbiguousWorkspace", err)
	}
	got, err := selectSlackWorkspace(conns, "T2")
	if err != nil || got.ID != "conn-2" {
		t.Fatalf("explicit team selected %#v, %v", got, err)
	}
}

func TestSelectSlackWorkspaceKeepsSingleTeamConvenience(t *testing.T) {
	conns := []*types.ServiceConnection{
		{ID: "manual", SlackTeamID: "T1"},
		{ID: "oauth", SlackTeamID: "T1", SlackAppConnectionID: "app-1"},
	}
	got, err := selectSlackWorkspace(conns, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "oauth" {
		t.Fatalf("selected %q, want OAuth connection", got.ID)
	}
}

func TestSlackWorkspaceByTeamIDRejectsLegacyDuplicates(t *testing.T) {
	store := &slackWorkspaceTestStore{connections: []*types.ServiceConnection{
		{ID: "conn-1", OrganizationID: "org-1", SlackTeamID: "T1"},
		{ID: "conn-2", OrganizationID: "org-2", SlackTeamID: "T1"},
	}}
	workspaces := newSlackWorkspaces(store, func() ([]byte, error) { return make([]byte, 32), nil })

	if _, err := workspaces.ByTeamID(context.Background(), "T1"); !errors.Is(err, slacktransport.ErrAmbiguousWorkspace) {
		t.Fatalf("error = %v, want ErrAmbiguousWorkspace", err)
	}
}

func TestUpsertSlackWorkspaceRejectsAnotherOrganization(t *testing.T) {
	t.Setenv("HELIX_ENCRYPTION_KEY", "slack-workspace-test")
	store := &slackWorkspaceTestStore{connections: []*types.ServiceConnection{
		{ID: "conn-1", OrganizationID: "org-1", SlackTeamID: "T1"},
	}}
	server := &HelixAPIServer{Store: store}

	err := server.upsertSlackWorkspace(context.Background(), "org-2", slackcore.Install{BotToken: "xoxb-new", TeamID: "T1"}, "")
	if !errors.Is(err, errSlackWorkspaceConflict) {
		t.Fatalf("error = %v, want errSlackWorkspaceConflict", err)
	}
	if err.Error() != "This Slack workspace is already connected to another Helix organization" {
		t.Fatalf("error = %q", err)
	}
}

func TestUpsertSlackWorkspaceRefreshesSameOrganization(t *testing.T) {
	t.Setenv("HELIX_ENCRYPTION_KEY", "slack-workspace-test")
	store := &slackWorkspaceTestStore{connections: []*types.ServiceConnection{
		{ID: "conn-1", OrganizationID: "org-1", SlackTeamID: "T1", SlackBotToken: "old"},
	}}
	server := &HelixAPIServer{Store: store, helixOrg: &helixOrgHandlers{}}

	err := server.upsertSlackWorkspace(context.Background(), "org-1", slackcore.Install{
		BotToken: "xoxb-new",
		TeamID:   "T1",
		TeamName: "Acme",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if store.updated == nil || store.updated.Name != "Acme" || store.updated.SlackBotToken == "old" {
		t.Fatalf("updated connection = %#v", store.updated)
	}
}

func TestSlackRedirectURITrimsTrailingSlash(t *testing.T) {
	server := &HelixAPIServer{Cfg: &config.ServerConfig{WebServer: config.WebServer{URL: "https://prime.helix.ml/"}}}
	if got := server.slackRedirectURI(); got != "https://prime.helix.ml/api/v1/slack/oauth/callback" {
		t.Fatalf("slackRedirectURI() = %q", got)
	}
}

func TestSlackOAuthCallbackRedirectsSlackErrorToSettings(t *testing.T) {
	t.Setenv("HELIX_ENCRYPTION_KEY", "slack-oauth-test")
	key, err := crypto.GetEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	state, err := crypto.EncryptAES256GCM([]byte("org-1|app-1"), key)
	if err != nil {
		t.Fatal(err)
	}
	server := &HelixAPIServer{Cfg: &config.ServerConfig{WebServer: config.WebServer{URL: "https://prime.helix.ml/"}}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/slack/oauth/callback?error=access_denied&state="+url.QueryEscape(state), nil)
	rec := httptest.NewRecorder()

	server.slackOAuthCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.Path != "/orgs/org-1/helix-org/settings" {
		t.Fatalf("redirect path = %q", location.Path)
	}
	if got := location.Query().Get("slack_error"); got != "Slack authorization was cancelled. Try connecting the workspace again when you are ready." {
		t.Fatalf("slack_error = %q", got)
	}
}

func TestSlackOAuthCallbackRejectsMissingState(t *testing.T) {
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/slack/oauth/callback", nil)
	rec := httptest.NewRecorder()

	server.slackOAuthCallback(rec, req)

	if rec.Code != http.StatusBadRequest || rec.Body.String() != "missing state\n" {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
}

func TestSlackOAuthCallbackRejectsInvalidState(t *testing.T) {
	t.Setenv("HELIX_ENCRYPTION_KEY", "slack-oauth-test")
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/slack/oauth/callback?state=invalid", nil)
	rec := httptest.NewRecorder()

	server.slackOAuthCallback(rec, req)

	if rec.Code != http.StatusBadRequest || rec.Body.String() != "invalid state\n" {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
}

func TestSlackOAuthCallbackRedirectsMissingCodeToSettings(t *testing.T) {
	t.Setenv("HELIX_ENCRYPTION_KEY", "slack-oauth-test")
	key, err := crypto.GetEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	state, err := crypto.EncryptAES256GCM([]byte("org-1|app-1"), key)
	if err != nil {
		t.Fatal(err)
	}
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/slack/oauth/callback?state="+url.QueryEscape(state), nil)
	rec := httptest.NewRecorder()

	server.slackOAuthCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.Path != "/orgs/org-1/helix-org/settings" {
		t.Fatalf("redirect path = %q", location.Path)
	}
	if got := location.Query().Get("slack_error"); got != "Slack did not return an authorization code. Try connecting the workspace again." {
		t.Fatalf("slack_error = %q", got)
	}
}

func TestSlackAuthTestErrorClassifiesSlackResponse(t *testing.T) {
	status, _ := slackAuthTestError(goslack.SlackErrorResponse{Err: "invalid_auth"})
	if status != http.StatusBadRequest {
		t.Fatalf("Slack API error status = %d", status)
	}
	status, _ = slackAuthTestError(errors.New("connection refused"))
	if status != http.StatusBadGateway {
		t.Fatalf("network error status = %d", status)
	}
}
