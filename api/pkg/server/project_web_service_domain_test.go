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

// addDomainTestServer wires the add-domain handler for a project owned by
// usr_owner with the web service enabled and the hostname free in
// vhost_routes. previous is what the claim history returns for it.
func addDomainTestServer(t *testing.T, project *types.Project, previous *types.VHostDomainClaim) (*HelixAPIServer, *types.VHostRoute, *types.VHostDomainClaim) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	mockStore.EXPECT().GetVHostRouteByHostname(gomock.Any(), "app.customer.example").Return(nil, store.ErrNotFound)
	mockStore.EXPECT().GetProjectWebServiceState(gomock.Any(), project.ID).Return(&types.ProjectWebServiceState{ProjectID: project.ID, Enabled: true, ContainerPort: 3000}, nil)
	if previous == nil {
		mockStore.EXPECT().GetLatestVHostDomainClaim(gomock.Any(), "app.customer.example").Return(nil, store.ErrNotFound)
	} else {
		mockStore.EXPECT().GetLatestVHostDomainClaim(gomock.Any(), "app.customer.example").Return(previous, nil)
	}
	created := &types.VHostRoute{}
	mockStore.EXPECT().CreateVHostRoute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, r *types.VHostRoute) error { *created = *r; return nil })
	claim := &types.VHostDomainClaim{}
	mockStore.EXPECT().CreateVHostDomainClaim(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.VHostDomainClaim) error { *claim = *c; return nil })

	cfg := &config.ServerConfig{}
	cfg.WebServer.URL = "https://helix.example"
	return &HelixAPIServer{Cfg: cfg, Store: mockStore}, created, claim
}

func addDomain(t *testing.T, server *HelixAPIServer, projectID string) *types.VHostRoute {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/web-service/domains",
		strings.NewReader(`{"hostname":"App.Customer.Example"}`))
	req = mux.SetURLVars(req, map[string]string{"id": projectID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_owner"}))
	route, herr := server.addProjectWebServiceDomain(httptest.NewRecorder(), req)
	require.Nil(t, herr)
	return route
}

func TestAddDomainFirstClaimNeedsNoOwnershipProof(t *testing.T) {
	project := &types.Project{ID: "prj_a", UserID: "usr_owner"}
	server, created, claim := addDomainTestServer(t, project, nil)

	route := addDomain(t, server, "prj_a")
	require.False(t, route.OwnershipProofRequired)
	require.False(t, created.OwnershipProofRequired)
	require.NotEmpty(t, created.VerificationToken)
	require.Equal(t, "app.customer.example", claim.Hostname)
	require.Equal(t, "prj_a", claim.TargetID)
	require.Equal(t, types.VHostTargetProjectWebService, claim.TargetKind)
}

func TestAddDomainSameProjectReclaimNeedsNoOwnershipProof(t *testing.T) {
	project := &types.Project{ID: "prj_a", UserID: "usr_owner"}
	server, created, _ := addDomainTestServer(t, project, &types.VHostDomainClaim{
		Hostname: "app.customer.example", TargetID: "prj_a", OrganizationID: "",
	})

	route := addDomain(t, server, "prj_a")
	require.False(t, route.OwnershipProofRequired)
	require.False(t, created.OwnershipProofRequired)
}

// A hostname that another project held before may still point at Helix
// through its old CNAME, so the new claim must prove DNS control.
func TestAddDomainReclaimByOtherProjectRequiresOwnershipProof(t *testing.T) {
	project := &types.Project{ID: "prj_b", UserID: "usr_owner"}
	server, created, claim := addDomainTestServer(t, project, &types.VHostDomainClaim{
		Hostname: "app.customer.example", TargetID: "prj_a", OrganizationID: "",
	})

	route := addDomain(t, server, "prj_b")
	require.True(t, route.OwnershipProofRequired)
	require.True(t, created.OwnershipProofRequired)
	require.Equal(t, "prj_b", claim.TargetID)
}

func TestAddDomainReclaimByOtherOrganizationRequiresOwnershipProof(t *testing.T) {
	// Same project ID (a move between organizations) still counts as a
	// different holder.
	project := &types.Project{ID: "prj_a", UserID: "usr_owner"}
	server, created, _ := addDomainTestServer(t, project, &types.VHostDomainClaim{
		Hostname: "app.customer.example", TargetID: "prj_a", OrganizationID: "org_other",
	})

	route := addDomain(t, server, "prj_a")
	require.True(t, route.OwnershipProofRequired)
	require.True(t, created.OwnershipProofRequired)
}

func verificationRouter(server *HelixAPIServer) http.Handler {
	router := mux.NewRouter()
	router.HandleFunc("/.well-known/helix-domain-verify/{token}", server.domainVerificationResponse).Methods(http.MethodGet)
	return router
}

func TestDomainVerificationResponseEchoesOnlyTheHostsOwnToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetVHostRouteByHostname(gomock.Any(), "app.customer.example").Return(&types.VHostRoute{
		Hostname: "app.customer.example", TargetKind: types.VHostTargetProjectWebService, TargetID: "prj_a", VerificationToken: "tok_a",
	}, nil).AnyTimes()
	mockStore.EXPECT().GetVHostRouteByHostname(gomock.Any(), "stale.customer.example").Return(nil, store.ErrNotFound).AnyTimes()
	mockStore.EXPECT().GetVHostRouteByHostname(gomock.Any(), "share-x.dev.helix.example").Return(&types.VHostRoute{
		Hostname: "share-x.dev.helix.example", TargetKind: types.VHostTargetSandboxPreview, TargetID: "ses_1", VerificationToken: "tok_p",
	}, nil).AnyTimes()
	handler := verificationRouter(&HelixAPIServer{Store: mockStore})

	get := func(host, token string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://"+host+"/.well-known/helix-domain-verify/"+token, nil)
		handler.ServeHTTP(rec, req)
		return rec
	}

	own := get("app.customer.example:80", "tok_a")
	require.Equal(t, http.StatusOK, own.Code)
	require.Equal(t, "tok_a", own.Body.String())

	// Another project's token is not echoed on this host, and no token is
	// echoed on a host with no route — Helix answering for a stale CNAME
	// must not satisfy a claim.
	require.Equal(t, http.StatusNotFound, get("app.customer.example", "tok_b").Code)
	require.Equal(t, http.StatusNotFound, get("stale.customer.example", "tok_a").Code)
	require.Equal(t, http.StatusNotFound, get("share-x.dev.helix.example", "tok_p").Code)
}
