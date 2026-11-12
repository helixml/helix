package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// domainTestServer wires the domain handlers for a project owned by
// usr_owner with the web service enabled. Routes created through the add
// handler are kept so the delete handler and the verifier echo can see them.
type domainTestServer struct {
	*HelixAPIServer
	routes map[string]*types.VHostRoute // by hostname
}

func newDomainTestServer(t *testing.T, project *types.Project) *domainTestServer {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	s := &domainTestServer{routes: map[string]*types.VHostRoute{}}
	mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil).AnyTimes()
	mockStore.EXPECT().GetProjectWebServiceState(gomock.Any(), project.ID).Return(&types.ProjectWebServiceState{ProjectID: project.ID, Enabled: true, ContainerPort: 3000}, nil).AnyTimes()
	mockStore.EXPECT().GetVHostRouteByHostname(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, hostname string) (*types.VHostRoute, error) {
			if route, ok := s.routes[hostname]; ok {
				return route, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()
	mockStore.EXPECT().GetVHostRouteByID(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) (*types.VHostRoute, error) {
			for _, route := range s.routes {
				if route.ID == id {
					return route, nil
				}
			}
			return nil, store.ErrNotFound
		}).AnyTimes()
	mockStore.EXPECT().CreateVHostRoute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, r *types.VHostRoute) error {
			r.ID = "vhr_" + r.Hostname + "_" + r.VerificationToken
			s.routes[r.Hostname] = r
			return nil
		}).AnyTimes()
	mockStore.EXPECT().DeleteVHostRoute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) error {
			for hostname, route := range s.routes {
				if route.ID == id {
					delete(s.routes, hostname)
				}
			}
			return nil
		}).AnyTimes()

	cfg := &config.ServerConfig{}
	cfg.WebServer.URL = "https://helix.example"
	s.HelixAPIServer = &HelixAPIServer{Cfg: cfg, Store: mockStore}
	return s
}

func (s *domainTestServer) addDomain(t *testing.T, projectID, hostname string) *types.VHostRoute {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/web-service/domains",
		strings.NewReader(`{"hostname":"`+hostname+`"}`))
	req = mux.SetURLVars(req, map[string]string{"id": projectID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_owner"}))
	route, herr := s.addProjectWebServiceDomain(httptest.NewRecorder(), req)
	require.Nil(t, herr)
	return route
}

func (s *domainTestServer) deleteDomain(t *testing.T, projectID, routeID string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/"+projectID+"/web-service/domains/"+routeID, nil)
	req = mux.SetURLVars(req, map[string]string{"id": projectID, "domain_id": routeID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_owner"}))
	_, herr := s.deleteProjectWebServiceDomain(httptest.NewRecorder(), req)
	require.Nil(t, herr)
}

func (s *domainTestServer) echo(host, token string) *httptest.ResponseRecorder {
	router := mux.NewRouter()
	router.HandleFunc("/.well-known/helix-domain-verify/{token}", s.domainVerificationResponse).Methods(http.MethodGet)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://"+host+"/.well-known/helix-domain-verify/"+token, nil))
	return rec
}

// Every added custom domain starts unverified with its own token, whether it
// is the first claim or a delete-and-re-add: there is no history that could
// make the DNS proof optional.
func TestAddDomainAlwaysStartsUnverifiedWithFreshToken(t *testing.T) {
	project := &types.Project{ID: "prj_a", UserID: "usr_owner"}
	server := newDomainTestServer(t, project)

	first := server.addDomain(t, "prj_a", "App.Customer.Example")
	require.Equal(t, "app.customer.example", first.Hostname)
	require.Nil(t, first.VerifiedAt)
	require.NotEmpty(t, first.VerificationToken)

	server.deleteDomain(t, "prj_a", first.ID)
	readded := server.addDomain(t, "prj_a", "app.customer.example")
	require.Nil(t, readded.VerifiedAt)
	require.NotEmpty(t, readded.VerificationToken)
	require.NotEqual(t, first.VerificationToken, readded.VerificationToken)

	// The old token is dead with its row; only the live claim's token is echoed.
	require.Equal(t, http.StatusNotFound, server.echo("app.customer.example", first.VerificationToken).Code)
	require.Equal(t, http.StatusOK, server.echo("app.customer.example", readded.VerificationToken).Code)
}

func TestDomainVerificationResponseEchoesOnlyTheHostsOwnToken(t *testing.T) {
	project := &types.Project{ID: "prj_a", UserID: "usr_owner"}
	server := newDomainTestServer(t, project)
	route := server.addDomain(t, "prj_a", "app.customer.example")
	server.routes["share-x.dev.helix.example"] = &types.VHostRoute{
		ID: "vhr_p", Hostname: "share-x.dev.helix.example", TargetKind: types.VHostTargetSandboxPreview, TargetID: "ses_1", VerificationToken: "tok_p",
	}

	own := server.echo("app.customer.example:80", route.VerificationToken)
	require.Equal(t, http.StatusOK, own.Code)
	require.Equal(t, route.VerificationToken, own.Body.String())

	// Another token is not echoed on this host, no token is echoed on a host
	// with no route (Helix answering for a stale CNAME must not satisfy a
	// claim), and non-custom-domain rows never echo.
	require.Equal(t, http.StatusNotFound, server.echo("app.customer.example", "tok_other").Code)
	require.Equal(t, http.StatusNotFound, server.echo("stale.customer.example", route.VerificationToken).Code)
	require.Equal(t, http.StatusNotFound, server.echo("share-x.dev.helix.example", "tok_p").Code)
}

func TestVHostShouldIssueCertOnlyForVerifiedCustomDomains(t *testing.T) {
	project := &types.Project{ID: "prj_a", UserID: "usr_owner"}
	server := newDomainTestServer(t, project)
	cfg := parseVHostConfig("dev.helix.example", "https://helix.example")
	pending := server.addDomain(t, "prj_a", "app.customer.example")

	require.NoError(t, server.vhostShouldIssueCert(context.Background(), cfg, "helix.example"), "canonical host")
	require.Error(t, server.vhostShouldIssueCert(context.Background(), cfg, "app.customer.example"), "pending custom domain")
	require.Error(t, server.vhostShouldIssueCert(context.Background(), cfg, "unknown.customer.example"), "unregistered host")

	verifiedAt := pending.CreatedAt
	pending.VerifiedAt = &verifiedAt
	require.NoError(t, server.vhostShouldIssueCert(context.Background(), cfg, "app.customer.example"), "verified custom domain")

	server.routes["myproj.dev.helix.example"] = &types.VHostRoute{
		Hostname: "myproj.dev.helix.example", TargetKind: types.VHostTargetProjectWebService, TargetID: "prj_a", IsDefault: true, VerifiedAt: &verifiedAt,
	}
	require.NoError(t, server.vhostShouldIssueCert(context.Background(), cfg, "myproj.dev.helix.example"), "default subdomain")
}
