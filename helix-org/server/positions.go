package server

import (
	"net/http"

	"github.com/helixml/helix-org/domain"
)

type positionAttributes struct {
	RoleID   domain.RoleID      `json:"roleId"`
	ParentID *domain.PositionID `json:"parentId"`
}

func positionResource(p domain.Position) Resource {
	return Resource{
		Type: "positions",
		ID:   string(p.ID),
		Attributes: mustAttributes(positionAttributes{
			RoleID:   p.RoleID,
			ParentID: p.ParentID,
		}),
	}
}

func (s *Server) listPositions(w http.ResponseWriter, r *http.Request) {
	positions, err := s.store.Positions.List(r.Context())
	if err != nil {
		writeStoreError(w, err, "list positions")
		return
	}
	out := make([]Resource, 0, len(positions))
	for _, p := range positions {
		out = append(out, positionResource(p))
	}
	writeCollection(w, http.StatusOK, out)
}

func (s *Server) getPosition(w http.ResponseWriter, r *http.Request) {
	id := domain.PositionID(r.PathValue("id"))
	p, err := s.store.Positions.Get(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "get position")
		return
	}
	writeResource(w, http.StatusOK, positionResource(p))
}

func (s *Server) listPositionChildren(w http.ResponseWriter, r *http.Request) {
	id := domain.PositionID(r.PathValue("id"))
	positions, err := s.store.Positions.ListChildren(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "list children")
		return
	}
	out := make([]Resource, 0, len(positions))
	for _, p := range positions {
		out = append(out, positionResource(p))
	}
	writeCollection(w, http.StatusOK, out)
}
