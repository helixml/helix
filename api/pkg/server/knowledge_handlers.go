package server

import (
	"net/http"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *HelixAPIServer) listKnowledge(_ http.ResponseWriter, r *http.Request) ([]*types.Knowledge, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)

	appID := r.URL.Query().Get("app_id")

	knowledge, err := s.Store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		Owner:     user.ID,
		OwnerType: user.Type,
		AppID:     appID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return knowledge, nil
}
