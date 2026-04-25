package server

import (
	"net/http"
	"time"

	"github.com/helixml/helix-org/domain"
)

type roleAttributes struct {
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func roleResource(r domain.Role) Resource {
	return Resource{
		Type: "roles",
		ID:   string(r.ID),
		Attributes: mustAttributes(roleAttributes{
			Content:   r.Content,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		}),
	}
}

func (s *Server) listRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.store.Roles.List(r.Context())
	if err != nil {
		writeStoreError(w, err, "list roles")
		return
	}
	out := make([]Resource, 0, len(roles))
	for _, role := range roles {
		out = append(out, roleResource(role))
	}
	writeCollection(w, http.StatusOK, out)
}

func (s *Server) getRole(w http.ResponseWriter, r *http.Request) {
	id := domain.RoleID(r.PathValue("id"))
	role, err := s.store.Roles.Get(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "get role")
		return
	}
	writeResource(w, http.StatusOK, roleResource(role))
}
