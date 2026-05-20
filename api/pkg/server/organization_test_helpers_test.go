package server

import (
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

func expectResolveOrganizationByID(mockStore *store.MockStore, orgID string) *gomock.Call {
	return mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{
		ID: orgID,
	}).Return(&types.Organization{ID: orgID}, nil)
}
