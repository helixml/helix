package server

import (
	"net/http"
	"time"

	"github.com/helixml/helix-org/domain"
)

type environmentAttributes struct {
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Server) getEnvironment(w http.ResponseWriter, r *http.Request) {
	id := domain.WorkerID(r.PathValue("id"))
	env, err := s.store.Environments.Get(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "get environment")
		return
	}
	writeResource(w, http.StatusOK, Resource{
		Type: "environments",
		ID:   string(env.WorkerID),
		Attributes: mustAttributes(environmentAttributes{
			Path:      env.Path,
			CreatedAt: env.CreatedAt,
		}),
	})
}
