package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/interfaces/jsonapi"
)

// ProcessorOutputDTO is one output branch on the wire.
type ProcessorOutputDTO struct {
	TopicID string `json:"topic_id"`
	Match   string `json:"match,omitempty"`
	Label   string `json:"label,omitempty"`
	Owned   bool   `json:"owned"`
	// ManagedFor is set when this route is auto-managed by a reconciler for
	// the named Worker (the Slack auto-router). Empty for human-authored
	// routes. Read-only — the UI surfaces it; reconcilers own these routes.
	ManagedFor string `json:"managed_for,omitempty"`
}

// ProcessorAttributes is the JSON:API attributes object for a
// `processors` resource (response).
type ProcessorAttributes struct {
	Name         string               `json:"name"`
	InputTopicID string               `json:"input_topic_id"`
	Kind         string               `json:"kind"`
	Config       json.RawMessage      `json:"config,omitempty"`
	Outputs      []ProcessorOutputDTO `json:"outputs"`
	CreatedBy    string               `json:"created_by,omitempty"`
	CreatedAt    string               `json:"created_at,omitempty"`
	// Automated marks an automation-created processor (the Slack auto-router)
	// rather than a human-created one. Read-only provenance flag.
	Automated bool `json:"automated"`
}

// --- Request DTOs (swagger only) ---------------------------------------
// These mirror the JSON:API request documents so the generated TS client
// gets a typed payload argument. Handlers bind through jsonapi.Bind into
// processorWriteAttributes below; this wrapper exists purely to shape the
// OpenAPI schema. Config is an open object ({"template": "..."} for the
// template kind).

// ProcessorWriteRequest is the JSON:API body for create + update.
type ProcessorWriteRequest struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Name         string                 `json:"name"`
			InputTopicID string                 `json:"input_topic_id"`
			Kind         string                 `json:"kind"`
			Config       map[string]interface{} `json:"config,omitempty"`
			CreatedBy    string                 `json:"created_by,omitempty"`
			Outputs      []ProcessorOutputDTO   `json:"outputs,omitempty"`
		} `json:"attributes"`
	} `json:"data"`
}

// processorWriteAttributes is the request attributes for create/update.
type processorWriteAttributes struct {
	Name string `json:"name"`
	// Pointer so update can tell "omitted" (nil, leave unchanged) from
	// "" (disconnect the input). On create, nil/empty both mean no input.
	InputTopicID *string         `json:"input_topic_id"`
	Kind         string          `json:"kind"`
	Config       json.RawMessage `json:"config"`
	CreatedBy    string          `json:"created_by"`
	Outputs      []struct {
		TopicID string `json:"topic_id"`
		Match   string `json:"match"`
		Label   string `json:"label"`
	} `json:"outputs"`
}

func processorResource(p processor.Processor) jsonapi.Resource {
	outs := make([]ProcessorOutputDTO, 0, len(p.Outputs))
	for _, o := range p.Outputs {
		outs = append(outs, ProcessorOutputDTO{
			TopicID: string(o.TopicID), Match: o.Match, Label: o.Label, Owned: o.Owned, ManagedFor: o.ManagedFor,
		})
	}
	return jsonapi.Resource{
		Type: "processors",
		ID:   string(p.ID),
		Attributes: ProcessorAttributes{
			Name:         p.Name,
			InputTopicID: string(p.InputTopicID),
			Kind:         string(p.Kind),
			Config:       p.Config,
			Outputs:      outs,
			CreatedBy:    p.CreatedBy,
			CreatedAt:    p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Automated:    p.Automated(),
		},
	}
}

// procErrStatus maps a service error to an HTTP status: not-found→404,
// cycle→409, anything else from a write→400 (validation / bad input).
func procErrStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, processors.ErrCycle), errors.Is(err, store.ErrConflict):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}

func (a *apiHandler) requireProcessors(w http.ResponseWriter) bool {
	if a.deps.Processors == nil {
		jsonapi.WriteError(w, http.StatusServiceUnavailable, errors.New("processors service not configured"))
		return false
	}
	return true
}

// listProcessors lists every processor in the org.
//
// @Summary Helix-org: list processors
// @Tags HelixOrg
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/orgs/{org}/processors [get]
func (a *apiHandler) listProcessors(w http.ResponseWriter, r *http.Request) {
	if !a.requireProcessors(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		jsonapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	procs, err := a.deps.Processors.List(r.Context(), orgID)
	if err != nil {
		jsonapi.WriteError(w, http.StatusInternalServerError, fmt.Errorf("list processors: %w", err))
		return
	}
	resources := make([]jsonapi.Resource, 0, len(procs))
	for _, p := range procs {
		resources = append(resources, processorResource(p))
	}
	jsonapi.Write(w, http.StatusOK, jsonapi.NewDocument(resources, jsonapi.TotalMeta{Total: len(procs)}))
}

// createProcessor creates a processor, auto-provisioning its output
// topic(s).
//
// @Summary Helix-org: create a processor
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Param payload body api.ProcessorWriteRequest true "Processor spec"
// @Success 201 {object} map[string]interface{}
// @Router /api/v1/orgs/{org}/processors [post]
func (a *apiHandler) createProcessor(w http.ResponseWriter, r *http.Request) {
	if !a.requireProcessors(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		jsonapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	var attrs processorWriteAttributes
	if err := jsonapi.Bind(r, &attrs); err != nil {
		jsonapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	input := ""
	if attrs.InputTopicID != nil {
		input = *attrs.InputTopicID
	}
	p, err := a.deps.Processors.Create(r.Context(), orgID, processors.CreateParams{
		Name:         attrs.Name,
		InputTopicID: streaming.TopicID(input),
		Kind:         processor.Kind(attrs.Kind),
		Config:       attrs.Config,
		CreatedBy:    attrs.CreatedBy,
		Outputs:      toOutputSpecs(attrs),
	})
	if err != nil {
		jsonapi.WriteError(w, procErrStatus(err), fmt.Errorf("create processor: %w", err))
		return
	}
	jsonapi.Write(w, http.StatusCreated, jsonapi.NewDocument(processorResource(p)))
}

// getProcessor returns one processor.
//
// @Summary Helix-org: get a processor
// @Tags HelixOrg
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Param id path string true "Processor ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/orgs/{org}/processors/{id} [get]
func (a *apiHandler) getProcessor(w http.ResponseWriter, r *http.Request) {
	if !a.requireProcessors(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		jsonapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	id := processor.ProcessorID(r.PathValue("id"))
	if id == "" {
		jsonapi.WriteError(w, http.StatusBadRequest, errors.New("processor id is required"))
		return
	}
	p, err := a.deps.Processors.Get(r.Context(), orgID, id)
	if err != nil {
		jsonapi.WriteError(w, errStatus(err), fmt.Errorf("get processor %s: %w", id, err))
		return
	}
	jsonapi.Write(w, http.StatusOK, jsonapi.NewDocument(processorResource(p)))
}

// updateProcessor updates name/kind/config on a processor.
//
// @Summary Helix-org: update a processor
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Param id path string true "Processor ID"
// @Param payload body api.ProcessorWriteRequest true "Processor spec"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/orgs/{org}/processors/{id} [put]
func (a *apiHandler) updateProcessor(w http.ResponseWriter, r *http.Request) {
	if !a.requireProcessors(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		jsonapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	id := processor.ProcessorID(r.PathValue("id"))
	if id == "" {
		jsonapi.WriteError(w, http.StatusBadRequest, errors.New("processor id is required"))
		return
	}
	var attrs processorWriteAttributes
	if err := jsonapi.Bind(r, &attrs); err != nil {
		jsonapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	var inputPtr *streaming.TopicID
	if attrs.InputTopicID != nil {
		tid := streaming.TopicID(*attrs.InputTopicID)
		inputPtr = &tid
	}
	p, err := a.deps.Processors.Update(r.Context(), orgID, id, processors.UpdateParams{
		Name:         attrs.Name,
		Kind:         processor.Kind(attrs.Kind),
		Config:       attrs.Config,
		InputTopicID: inputPtr,
	})
	if err != nil {
		jsonapi.WriteError(w, procErrStatus(err), fmt.Errorf("update processor %s: %w", id, err))
		return
	}
	jsonapi.Write(w, http.StatusOK, jsonapi.NewDocument(processorResource(p)))
}

// deleteProcessor deletes a processor and its auto-provisioned output
// topics.
//
// @Summary Helix-org: delete a processor
// @Tags HelixOrg
// @Param org path string true "Organization ID or slug"
// @Param id path string true "Processor ID"
// @Success 204 "No Content"
// @Router /api/v1/orgs/{org}/processors/{id} [delete]
func (a *apiHandler) deleteProcessor(w http.ResponseWriter, r *http.Request) {
	if !a.requireProcessors(w) {
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		jsonapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	id := processor.ProcessorID(r.PathValue("id"))
	if id == "" {
		jsonapi.WriteError(w, http.StatusBadRequest, errors.New("processor id is required"))
		return
	}
	if err := a.deps.Processors.Delete(r.Context(), orgID, id); err != nil {
		jsonapi.WriteError(w, errStatus(err), fmt.Errorf("delete processor %s: %w", id, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toOutputSpecs(attrs processorWriteAttributes) []processors.OutputSpec {
	if len(attrs.Outputs) == 0 {
		return nil
	}
	out := make([]processors.OutputSpec, 0, len(attrs.Outputs))
	for _, o := range attrs.Outputs {
		out = append(out, processors.OutputSpec{TopicID: streaming.TopicID(o.TopicID), Match: o.Match, Label: o.Label})
	}
	return out
}
