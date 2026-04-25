package server

import (
	"net/http"

	"github.com/helixml/helix-org/domain"
)

type grantAttributes struct {
	WorkerID domain.WorkerID `json:"workerId"`
	ToolName domain.ToolName `json:"toolName"`
}

func grantResource(g domain.ToolGrant) Resource {
	return Resource{
		Type: "grants",
		ID:   string(g.ID),
		Attributes: mustAttributes(grantAttributes{
			WorkerID: g.WorkerID,
			ToolName: g.ToolName,
		}),
	}
}

func (s *Server) getGrant(w http.ResponseWriter, r *http.Request) {
	id := domain.GrantID(r.PathValue("id"))
	g, err := s.store.Grants.Get(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "get grant")
		return
	}
	writeResource(w, http.StatusOK, grantResource(g))
}
