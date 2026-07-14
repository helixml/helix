package api

import (
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/org/application/chartlayout"
)

// ChartPositionDTO is one free-placed node on the org chart canvas.
type ChartPositionDTO struct {
	// Kind is bot | topic | processor (matches the ReactFlow node id prefix).
	Kind string  `json:"kind"`
	ID   string  `json:"id"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

// ChartPositionsResponse is the body of GET /chart/positions.
type ChartPositionsResponse struct {
	Positions []ChartPositionDTO `json:"positions"`
}

// UpsertChartPositionsRequest is the body of PUT /chart/positions.
// Positions is a full upsert set for the nodes listed — other saved
// positions are left alone.
type UpsertChartPositionsRequest struct {
	Positions []ChartPositionDTO `json:"positions"`
}

// getChartPositions returns every free-placed canvas coordinate for the org.
//
// @Summary Helix-org: list chart node positions
// @Description Returns free-placed (x, y) coordinates for org-chart nodes. Nodes without a row fall back to auto-layout.
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.ChartPositionsResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/chart/positions [get]
func (a *apiHandler) getChartPositions(w http.ResponseWriter, r *http.Request) {
	if a.deps.ChartLayout == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("chart layout not wired"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rows, err := a.deps.ChartLayout.List(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list chart positions: %w", err))
		return
	}
	out := ChartPositionsResponse{Positions: make([]ChartPositionDTO, 0, len(rows))}
	for _, p := range rows {
		out.Positions = append(out.Positions, ChartPositionDTO{
			Kind: p.Kind,
			ID:   p.ID,
			X:    p.X,
			Y:    p.Y,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// putChartPositions upserts free-placed canvas coordinates.
//
// @Summary Helix-org: upsert chart node positions
// @Description Upserts (x, y) coordinates for one or more org-chart nodes after the user drags them.
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param body body api.UpsertChartPositionsRequest true "positions to save"
// @Success 200 {object} api.ChartPositionsResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/chart/positions [put]
func (a *apiHandler) putChartPositions(w http.ResponseWriter, r *http.Request) {
	if a.deps.ChartLayout == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("chart layout not wired"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req UpsertChartPositionsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	inputs := make([]chartlayout.PositionInput, 0, len(req.Positions))
	for _, p := range req.Positions {
		inputs = append(inputs, chartlayout.PositionInput{
			Kind: p.Kind,
			ID:   p.ID,
			X:    p.X,
			Y:    p.Y,
		})
	}
	saved, err := a.deps.ChartLayout.Upsert(r.Context(), orgID, inputs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out := ChartPositionsResponse{Positions: make([]ChartPositionDTO, 0, len(saved))}
	for _, p := range saved {
		out.Positions = append(out.Positions, ChartPositionDTO{
			Kind: p.Kind,
			ID:   p.ID,
			X:    p.X,
			Y:    p.Y,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// deleteChartPositions clears every free-placed coordinate for the org
// so the chart falls back to auto-layout on the next load.
//
// @Summary Helix-org: reset chart layout
// @Description Deletes every saved node position for the org; the chart reverts to auto-layout.
// @Tags HelixOrg
// @Success 204 "no content"
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/chart/positions [delete]
func (a *apiHandler) deleteChartPositions(w http.ResponseWriter, r *http.Request) {
	if a.deps.ChartLayout == nil {
		writeError(w, http.StatusServiceUnavailable, fmt.Errorf("chart layout not wired"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.deps.ChartLayout.Clear(r.Context(), orgID); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("clear chart positions: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
