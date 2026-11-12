package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetOrganizationGuidelinesHistory_EnforcesKeyOrgScope(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).
		Return(&types.Organization{ID: "org_b", Name: "b"}, nil).AnyTimes()
	// The user really is a member of org_b; only the key's scope differs.
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(&types.OrganizationMembership{OrganizationID: "org_b", UserID: "user-1", Role: types.OrganizationRoleMember}, nil).AnyTimes()
	// ListGuidelinesHistory must NOT be called for the out-of-scope key.

	server := &HelixAPIServer{Store: mockStore}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/org_b/guidelines-history", nil)
	r = mux.SetURLVars(r, map[string]string{"id": "org_b"})
	r = r.WithContext(setRequestUser(r.Context(), types.User{
		ID:             "user-1",
		TokenType:      types.TokenTypeAPIKey,
		OrganizationID: "org_a",
	}))
	rec := httptest.NewRecorder()

	server.getOrganizationGuidelinesHistory(rec, r)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
