package server

import (
	"net/http"

	"github.com/helixml/helix-org/domain"
)

type workerAttributes struct {
	Kind      domain.WorkerKind   `json:"kind"`
	Positions []domain.PositionID `json:"positions"`
}

func workerResource(w domain.Worker) Resource {
	return Resource{
		Type: "workers",
		ID:   string(w.ID()),
		Attributes: mustAttributes(workerAttributes{
			Kind:      w.Kind(),
			Positions: w.Positions(),
		}),
	}
}

func (s *Server) listWorkers(w http.ResponseWriter, r *http.Request) {
	workers, err := s.store.Workers.List(r.Context())
	if err != nil {
		writeStoreError(w, err, "list workers")
		return
	}
	out := make([]Resource, 0, len(workers))
	for _, worker := range workers {
		out = append(out, workerResource(worker))
	}
	writeCollection(w, http.StatusOK, out)
}

func (s *Server) getWorker(w http.ResponseWriter, r *http.Request) {
	id := domain.WorkerID(r.PathValue("id"))
	worker, err := s.store.Workers.Get(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "get worker")
		return
	}
	writeResource(w, http.StatusOK, workerResource(worker))
}

func (s *Server) listWorkerGrants(w http.ResponseWriter, r *http.Request) {
	id := domain.WorkerID(r.PathValue("id"))
	grants, err := s.store.Grants.ListByWorker(r.Context(), id)
	if err != nil {
		writeStoreError(w, err, "list grants")
		return
	}
	out := make([]Resource, 0, len(grants))
	for _, g := range grants {
		out = append(out, grantResource(g))
	}
	writeCollection(w, http.StatusOK, out)
}
