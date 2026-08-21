package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
)

var errWorkerSecretsUnavailable = errors.New("worker secrets are unavailable")

type WorkerSecretBindingDTO struct {
	Name              string                  `json:"name"`
	Description       string                  `json:"description,omitempty"`
	Usage             string                  `json:"usage,omitempty"`
	ContentType       string                  `json:"content_type,omitempty"`
	SuggestedFilename string                  `json:"suggested_filename,omitempty"`
	SourceKind        workersecret.SourceKind `json:"source_kind"`
	SecretID          string                  `json:"secret_id,omitempty"`
	AccountID         string                  `json:"account_id,omitempty"`
	ExportKey         string                  `json:"export_key,omitempty"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
}
type PutWorkerSecretRequest struct {
	Description       string                  `json:"description,omitempty"`
	Usage             string                  `json:"usage,omitempty"`
	ContentType       string                  `json:"content_type,omitempty"`
	SuggestedFilename string                  `json:"suggested_filename,omitempty"`
	SourceKind        workersecret.SourceKind `json:"source_kind"`
	SecretID          string                  `json:"secret_id,omitempty"`
	AccountID         string                  `json:"account_id,omitempty"`
	ExportKey         string                  `json:"export_key,omitempty"`
}

func bindingDTO(b workersecret.Binding) WorkerSecretBindingDTO {
	return WorkerSecretBindingDTO{Name: b.Name, Description: b.Description, Usage: b.Usage, ContentType: b.ContentType, SuggestedFilename: b.SuggestedFilename, SourceKind: b.SourceKind, SecretID: b.SecretID, AccountID: b.AccountID, ExportKey: b.ExportKey, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt}
}

// @Summary List an Agent's secret bindings
// @Tags HelixOrg
// @Success 200 {array} api.WorkerSecretBindingDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/secrets [get]
func (a *apiHandler) listWorkerSecrets(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if a.deps.WorkerSecrets == nil {
		writeError(w, 503, errWorkerSecretsUnavailable)
		return
	}
	rows, err := a.deps.WorkerSecrets.List(r.Context(), orgID, orgchart.NodeID(r.PathValue("id")))
	if err != nil {
		writeError(w, errStatus(err), err)
		return
	}
	out := make([]WorkerSecretBindingDTO, len(rows))
	for i := range rows {
		out[i] = bindingDTO(rows[i])
	}
	writeJSON(w, 200, out)
}

// @Summary List sources that may be granted to an Agent
// @Tags HelixOrg
// @Success 200 {array} workersecret.AvailableSource
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/available-secrets [get]
func (a *apiHandler) listAvailableWorkerSecrets(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if a.deps.WorkerSecrets == nil {
		writeError(w, 503, errWorkerSecretsUnavailable)
		return
	}
	rows, err := a.deps.WorkerSecrets.Available(r.Context(), orgID, orgchart.NodeID(r.PathValue("id")))
	if err != nil {
		writeError(w, errStatus(err), err)
		return
	}
	writeJSON(w, 200, rows)
}

// @Summary Create or replace an Agent secret binding
// @Tags HelixOrg
// @Param payload body api.PutWorkerSecretRequest true "Binding metadata"
// @Success 200 {object} api.WorkerSecretBindingDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/secrets/{name} [put]
func (a *apiHandler) putWorkerSecret(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if a.deps.WorkerSecrets == nil {
		writeError(w, 503, errWorkerSecretsUnavailable)
		return
	}
	var req PutWorkerSecretRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, 400, err)
		return
	}
	b, err := a.deps.WorkerSecrets.Put(r.Context(), workersecret.Binding{OrganizationID: orgID, WorkerID: orgchart.NodeID(r.PathValue("id")), Name: r.PathValue("name"), Description: req.Description, Usage: req.Usage, ContentType: req.ContentType, SuggestedFilename: req.SuggestedFilename, SourceKind: req.SourceKind, SecretID: req.SecretID, AccountID: req.AccountID, ExportKey: req.ExportKey})
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, 200, bindingDTO(b))
}

// @Summary Delete an Agent secret binding
// @Tags HelixOrg
// @Success 204
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/secrets/{name} [delete]
func (a *apiHandler) deleteWorkerSecret(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	if a.deps.WorkerSecrets == nil {
		writeError(w, 503, errWorkerSecretsUnavailable)
		return
	}
	if err := a.deps.WorkerSecrets.Delete(r.Context(), orgID, orgchart.NodeID(r.PathValue("id")), r.PathValue("name")); err != nil {
		writeError(w, errStatus(err), err)
		return
	}
	w.WriteHeader(204)
}
