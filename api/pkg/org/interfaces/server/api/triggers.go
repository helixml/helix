package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	triggerapp "github.com/helixml/helix/api/pkg/org/application/triggers"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
)

type APIError struct {
	Code          string         `json:"code"`
	Summary       string         `json:"summary"`
	Field         string         `json:"field,omitempty"`
	Resource      map[string]any `json:"resource,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
}

type SourceRefDTO struct {
	Kind        string `json:"kind"`
	TriggerID   string `json:"trigger_id,omitempty"`
	ProcessorID string `json:"processor_id,omitempty"`
	OutputID    string `json:"output_id,omitempty"`
}

type TriggerDTO struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Kind        string         `json:"kind"`
	Config      map[string]any `json:"config,omitempty"`
	CreatedAt   string         `json:"created_at"`
	Revision    string         `json:"revision"`
	// AttachedWorkers are the Workers this Trigger activates — the
	// attachment-model successor of the Topics page's subscriber list.
	AttachedWorkers []string `json:"attached_workers,omitempty"`
	// EffectivePublicURL is helix's public base URL (SERVER_URL), set
	// only for provider Triggers whose webhook payload URL must be
	// reachable from the internet, so the UI can warn on loopback.
	EffectivePublicURL string `json:"effective_public_url,omitempty"`
	// Activation is the resolved "how do I fire this" recipe for this
	// Trigger: concrete URL or address, verb, and auth, with every
	// template in the Kind's descriptor filled in.
	Activation *transport.ResolvedActivation `json:"activation,omitempty"`
}

type TriggerWriteRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Kind        string         `json:"kind"`
	Config      map[string]any `json:"config,omitempty"`
	Revision    string         `json:"revision,omitempty"`
}

type TriggerListResponse struct {
	Triggers []TriggerDTO `json:"triggers"`
}

type AttachmentDTO struct {
	ID        string       `json:"id"`
	WorkerID  string       `json:"worker_id"`
	Source    SourceRefDTO `json:"source"`
	CreatedAt string       `json:"created_at"`
}

type AttachmentWriteRequest struct {
	Source SourceRefDTO `json:"source"`
}

type AttachmentListResponse struct {
	Attachments []AttachmentDTO `json:"attachments"`
}

type TriggerEventDTO struct {
	ID        string `json:"id"`
	Source    string `json:"source,omitempty"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type TriggerEventsResponse struct {
	Events []TriggerEventDTO `json:"events"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

func writeAPIError(w http.ResponseWriter, status int, code, summary string) {
	writeJSON(w, status, APIError{Code: code, Summary: summary})
}

func (a *apiHandler) triggerDTO(ctx context.Context, orgID string, t trigger.Trigger) TriggerDTO {
	var config map[string]any
	if len(t.Config) > 0 {
		_ = json.Unmarshal(t.Config, &config)
	}
	dto := TriggerDTO{ID: t.ID, Name: t.Name, Description: t.Description, Kind: string(t.Kind), Config: config, CreatedAt: t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"), Revision: triggerapp.Revision(t)}
	if t.Kind == transport.KindGitHub || t.Kind == transport.KindGitLab {
		dto.EffectivePublicURL = strings.TrimSpace(a.deps.PublicServerURL)
	}
	// The handle is what the caller put in the URL, so a copied
	// activation URL matches the address they navigated to. Both a slug
	// and an id resolve, so falling back to the id is safe.
	handle := helixorgserver.OrgHandleFromContext(ctx)
	if handle == "" {
		handle = orgID
	}
	for _, d := range transport.DescribeAll() {
		if d.Kind != t.Kind {
			continue
		}
		resolved := transport.ResolveActivation(d, transport.ActivationParams{
			PublicURL: strings.TrimSpace(a.deps.PublicServerURL),
			OrgHandle: handle,
			TriggerID: t.ID,
			Config:    config,
		})
		dto.Activation = &resolved
		break
	}
	// Attachments are a separate aggregate; a read failure here must
	// not fail the whole list, so the column just comes back empty.
	if a.deps.Queries != nil {
		if members, err := a.deps.Queries.TriggerMembers(ctx, orgID, t.ID); err == nil {
			for _, m := range members {
				dto.AttachedWorkers = append(dto.AttachedWorkers, string(m.WorkerID))
			}
		}
	}
	return dto
}

type TriggerKindsResponse struct {
	Kinds []transport.Descriptor `json:"kinds"`
}

// @Summary Helix-org: list trigger kinds and their settings
// @Tags HelixOrg
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Success 200 {object} api.TriggerKindsResponse
// @Router /api/v1/orgs/{org}/trigger-kinds [get]
func (a *apiHandler) listTriggerKinds(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, TriggerKindsResponse{Kinds: transport.DescribeAll()})
}

func triggerConfig(config map[string]any) (json.RawMessage, error) {
	if len(config) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode trigger config: %w", err)
	}
	return raw, nil
}

func sourceRefDTO(s eventsource.SourceRef) SourceRefDTO {
	return SourceRefDTO{Kind: string(s.Kind), TriggerID: s.TriggerID, ProcessorID: s.ProcessorID, OutputID: s.OutputID}
}

func parseSourceRef(s SourceRefDTO) (eventsource.SourceRef, error) {
	r := eventsource.SourceRef{Kind: eventsource.Kind(s.Kind), TriggerID: s.TriggerID, ProcessorID: s.ProcessorID, OutputID: s.OutputID}
	return r, r.Validate()
}

func (a *apiHandler) requireTriggers(w http.ResponseWriter) bool {
	if a.deps.Triggers == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "Trigger service is unavailable")
		return false
	}
	return true
}

// @Summary Helix-org: list triggers
// @Tags HelixOrg
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Success 200 {object} api.TriggerListResponse
// @Router /api/v1/orgs/{org}/triggers [get]
func (a *apiHandler) listTriggers(w http.ResponseWriter, r *http.Request) {
	if !a.requireTriggers(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	rows, err := a.deps.Triggers.List(r.Context(), orgID)
	if err != nil {
		writeAPIError(w, 500, "internal_error", "Could not list triggers")
		return
	}
	out := make([]TriggerDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, a.triggerDTO(r.Context(), orgID, row))
	}
	writeJSON(w, http.StatusOK, TriggerListResponse{Triggers: out})
}

// @Summary Helix-org: create a trigger
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Param payload body api.TriggerWriteRequest true "Trigger"
// @Success 201 {object} api.TriggerDTO
// @Failure 400 {object} api.APIError
// @Router /api/v1/orgs/{org}/triggers [post]
func (a *apiHandler) createTrigger(w http.ResponseWriter, r *http.Request) {
	if !a.requireTriggers(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	var req TriggerWriteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	config, err := triggerConfig(req.Config)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	t, err := a.deps.Triggers.Create(r.Context(), orgID, triggerapp.CreateParams{Name: req.Name, Description: req.Description, Kind: transport.Kind(req.Kind), Config: config})
	if err != nil {
		code, status := "validation_failed", 400
		if errors.Is(err, store.ErrConflict) {
			code, status = "conflict", 409
		}
		writeAPIError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a.triggerDTO(r.Context(), orgID, t))
}

// @Summary Helix-org: get a trigger
// @Tags HelixOrg
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Param id path string true "Trigger ID"
// @Success 200 {object} api.TriggerDTO
// @Router /api/v1/orgs/{org}/triggers/{id} [get]
func (a *apiHandler) getTrigger(w http.ResponseWriter, r *http.Request) {
	if !a.requireTriggers(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	t, err := a.deps.Triggers.Get(r.Context(), orgID, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, 404, "not_found", "Trigger not found")
		return
	}
	writeJSON(w, http.StatusOK, a.triggerDTO(r.Context(), orgID, t))
}

// @Summary Helix-org: update a trigger
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Param id path string true "Trigger ID"
// @Param payload body api.TriggerWriteRequest true "Trigger"
// @Success 200 {object} api.TriggerDTO
// @Failure 409 {object} api.APIError
// @Router /api/v1/orgs/{org}/triggers/{id} [put]
func (a *apiHandler) updateTrigger(w http.ResponseWriter, r *http.Request) {
	if !a.requireTriggers(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	var req TriggerWriteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	revision := r.Header.Get("If-Match")
	if revision == "" {
		revision = req.Revision
	}
	config, err := triggerConfig(req.Config)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	t, err := a.deps.Triggers.Update(r.Context(), orgID, r.PathValue("id"), revision, triggerapp.UpdateParams{Name: req.Name, Description: req.Description, Kind: transport.Kind(req.Kind), Config: config})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeAPIError(w, 404, "not_found", "Trigger not found")
		} else if errors.Is(err, store.ErrConflict) {
			writeAPIError(w, 409, "stale_resource", "Trigger changed; refresh before saving")
		} else {
			writeAPIError(w, 400, "validation_failed", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, a.triggerDTO(r.Context(), orgID, t))
}

// @Summary Helix-org: delete a trigger
// @Tags HelixOrg
// @Param org path string true "Organization ID or slug"
// @Param id path string true "Trigger ID"
// @Success 204
// @Router /api/v1/orgs/{org}/triggers/{id} [delete]
func (a *apiHandler) deleteTrigger(w http.ResponseWriter, r *http.Request) {
	if !a.requireTriggers(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	err = a.deps.Triggers.Delete(r.Context(), orgID, r.PathValue("id"))
	if err != nil {
		if errors.Is(err, triggerapp.ErrSourceInUse) {
			writeAPIError(w, 409, "source_in_use", "Detach workers before deleting this trigger")
		} else {
			writeAPIError(w, 404, "not_found", "Trigger not found")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary Helix-org: list trigger events
// @Tags HelixOrg
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Param id path string true "Trigger ID"
// @Param limit query int false "Page size (1-100)"
// @Param offset query int false "Offset"
// @Success 200 {object} api.TriggerEventsResponse
// @Router /api/v1/orgs/{org}/triggers/{id}/events [get]
func (a *apiHandler) listTriggerEvents(w http.ResponseWriter, r *http.Request) {
	if !a.requireTriggers(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, err := a.deps.Triggers.Events(r.Context(), orgID, r.PathValue("id"), limit, offset)
	if err != nil {
		writeAPIError(w, 404, "not_found", "Trigger not found")
		return
	}
	out := make([]TriggerEventDTO, 0, len(rows))
	for _, e := range rows {
		out = append(out, eventDTO(e))
	}
	writeJSON(w, 200, TriggerEventsResponse{Events: out, Total: total, Limit: limit, Offset: offset})
}

func eventDTO(e streaming.Event) TriggerEventDTO {
	return TriggerEventDTO{ID: string(e.ID), Source: e.Source, Body: e.Body, CreatedAt: e.CreatedAt.Format("2006-01-02T15:04:05Z07:00")}
}

func (a *apiHandler) requireAttachments(w http.ResponseWriter) bool {
	if a.deps.Attachments == nil {
		writeAPIError(w, 503, "service_unavailable", "Attachment service is unavailable")
		return false
	}
	return true
}

// @Summary Helix-org: list agent attachments
// @Tags HelixOrg
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Param id path string true "Agent ID"
// @Success 200 {object} api.AttachmentListResponse
// @Router /api/v1/orgs/{org}/agents/{id}/attachments [get]
func (a *apiHandler) listAgentAttachments(w http.ResponseWriter, r *http.Request) {
	if !a.requireAttachments(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	rows, err := a.deps.Attachments.ListForWorker(r.Context(), orgID, orgchart.NodeID(r.PathValue("id")))
	if err != nil {
		writeAPIError(w, 404, "not_found", "Agent not found")
		return
	}
	out := make([]AttachmentDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, AttachmentDTO{ID: row.ID, WorkerID: string(row.WorkerID), Source: sourceRefDTO(row.Source), CreatedAt: row.CreatedAt.Format("2006-01-02T15:04:05Z07:00")})
	}
	writeJSON(w, 200, AttachmentListResponse{Attachments: out})
}

// @Summary Helix-org: attach an agent to a source
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Param id path string true "Agent ID"
// @Param payload body api.AttachmentWriteRequest true "Source"
// @Success 201 {object} api.AttachmentDTO
// @Router /api/v1/orgs/{org}/agents/{id}/attachments [post]
func (a *apiHandler) createAgentAttachment(w http.ResponseWriter, r *http.Request) {
	if !a.requireAttachments(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	var req AttachmentWriteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	source, err := parseSourceRef(req.Source)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	row, err := a.deps.Attachments.Create(r.Context(), orgID, orgchart.NodeID(r.PathValue("id")), source, "")
	if err != nil {
		status, code := 400, "validation_failed"
		if errors.Is(err, store.ErrConflict) {
			status, code = 409, "conflict"
		}
		if errors.Is(err, store.ErrNotFound) {
			status, code = 404, "not_found"
		}
		writeAPIError(w, status, code, fmt.Sprintf("Could not attach source: %v", err))
		return
	}
	writeJSON(w, 201, AttachmentDTO{ID: row.ID, WorkerID: string(row.WorkerID), Source: sourceRefDTO(row.Source), CreatedAt: row.CreatedAt.Format("2006-01-02T15:04:05Z07:00")})
}

// @Summary Helix-org: delete an agent attachment
// @Tags HelixOrg
// @Param org path string true "Organization ID or slug"
// @Param id path string true "Agent ID"
// @Param attachment_id path string true "Attachment ID"
// @Success 204
// @Router /api/v1/orgs/{org}/agents/{id}/attachments/{attachment_id} [delete]
func (a *apiHandler) deleteAgentAttachment(w http.ResponseWriter, r *http.Request) {
	if !a.requireAttachments(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeAPIError(w, 400, "validation_failed", err.Error())
		return
	}
	rows, err := a.deps.Attachments.ListForWorker(r.Context(), orgID, orgchart.NodeID(r.PathValue("id")))
	if err != nil {
		writeAPIError(w, 404, "not_found", "Attachment not found")
		return
	}
	id := r.PathValue("attachment_id")
	found := false
	for _, row := range rows {
		if row.ID == id {
			found = true
			break
		}
	}
	if !found {
		writeAPIError(w, 404, "not_found", "Attachment not found")
		return
	}
	if err := a.deps.Attachments.Delete(r.Context(), orgID, id); err != nil {
		writeAPIError(w, 404, "not_found", "Attachment not found")
		return
	}
	w.WriteHeader(204)
}
