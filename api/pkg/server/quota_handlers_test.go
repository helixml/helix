package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type recordingQuotaManager struct {
	got *types.QuotaRequest
}

func (m *recordingQuotaManager) GetQuotas(_ context.Context, req *types.QuotaRequest) (*types.QuotaResponse, error) {
	m.got = req
	return &types.QuotaResponse{MaxProjects: 3}, nil
}

func (m *recordingQuotaManager) LimitReached(context.Context, *types.QuotaLimitReachedRequest) (*types.QuotaLimitReachedResponse, error) {
	return &types.QuotaLimitReachedResponse{}, nil
}

func quotasRequest(url string, user *types.User) *http.Request {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	return req.WithContext(setTestRequestUser(req.Context(), user))
}

func TestGetQuotas_NoOrgReturnsUserQuotas(t *testing.T) {
	ctrl := gomock.NewController(t)
	qm := &recordingQuotaManager{}
	s := &HelixAPIServer{Store: store.NewMockStore(ctrl), quotaManager: qm}

	rec := httptest.NewRecorder()
	s.getQuotasHandler(rec, quotasRequest("/api/v1/quotas", &types.User{ID: "usr_1"}))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, &types.QuotaRequest{UserID: "usr_1"}, qm.got)
}

func TestGetQuotas_OrgSlugResolvesToOrgID(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := store.NewMockStore(ctrl)
	qm := &recordingQuotaManager{}
	s := &HelixAPIServer{Store: db, quotaManager: qm}

	db.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{Name: "acme"}).
		Return(&types.Organization{ID: "org_1", Name: "acme"}, nil)
	db.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{OrganizationID: "org_1", UserID: "usr_1"}).
		Return(&types.OrganizationMembership{OrganizationID: "org_1", UserID: "usr_1"}, nil)

	rec := httptest.NewRecorder()
	s.getQuotasHandler(rec, quotasRequest("/api/v1/quotas?org_id=acme", &types.User{ID: "usr_1"}))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, &types.QuotaRequest{UserID: "usr_1", OrganizationID: "org_1"}, qm.got)
}
