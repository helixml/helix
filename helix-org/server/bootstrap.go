package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/helixml/helix-org/bootstrap"
	"github.com/helixml/helix-org/domain"
)

type bootstrapAttributes struct {
	RoleID          domain.RoleID     `json:"roleId"`
	PositionID      domain.PositionID `json:"positionId"`
	EnvironmentPath string            `json:"environmentPath"`
}

// bootstrap creates the initial owner. The Environment lives at
// <envsDir>/w-owner; we create the directory here so the bootstrap
// package itself stays unaware of the system-wide envs convention.
// Body is empty — there is nothing for the caller to configure.
func (s *Server) bootstrap(w http.ResponseWriter, _ *http.Request) {
	if s.envsDir == "" {
		writeError(w, http.StatusInternalServerError, "no envs dir", "server was constructed without an envs directory")
		return
	}
	envPath := filepath.Join(s.envsDir, "w-owner")
	if err := os.MkdirAll(envPath, 0o750); err != nil {
		writeError(w, http.StatusInternalServerError, "create env dir", err.Error())
		return
	}

	s.logger.Info("bootstrap.start", "envPath", envPath)
	result, err := bootstrap.Run(context.Background(), s.store, bootstrap.Params{
		EnvironmentPath: envPath,
	})
	if err != nil {
		s.logger.Warn("bootstrap.fail", "err", err.Error())
		switch {
		case errors.Is(err, bootstrap.ErrAlreadyInitialised):
			writeError(w, http.StatusConflict, "already initialised", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "bootstrap failed", err.Error())
		}
		return
	}
	s.logger.Info("bootstrap.ok",
		"worker", result.WorkerID,
		"role", result.RoleID,
		"position", result.PositionID,
		"envPath", result.EnvironmentPath,
	)
	writeResource(w, http.StatusCreated, Resource{
		Type: "workers",
		ID:   string(result.WorkerID),
		Attributes: mustAttributes(bootstrapAttributes{
			RoleID:          result.RoleID,
			PositionID:      result.PositionID,
			EnvironmentPath: result.EnvironmentPath,
		}),
	})
}
