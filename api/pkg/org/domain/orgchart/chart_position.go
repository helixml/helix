package orgchart

import (
	"errors"
	"time"
)

// Chart node kinds stored in org_chart_positions. Values match the
// ReactFlow node id prefix used by the chart UI (bot:…, topic:…,
// processor:…).
const (
	ChartNodeKindBot       = "bot"
	ChartNodeKindTopic     = "topic"
	ChartNodeKindProcessor = "processor"
)

// ValidChartNodeKind reports whether kind is one of the chart node
// kinds the UI can place.
func ValidChartNodeKind(kind string) bool {
	switch kind {
	case ChartNodeKindBot, ChartNodeKindTopic, ChartNodeKindProcessor:
		return true
	default:
		return false
	}
}

// ChartPosition is one free-placed node on an org's chart canvas.
// Pure UI layout — not part of the org-graph domain model (bots /
// topics / processors remain the source of truth for structure).
// When no row exists for a node the chart falls back to auto-layout
// (dagre + topic columns).
type ChartPosition struct {
	OrganizationID string
	// Kind is bot | topic | processor.
	Kind string
	// ID is the entity handle (bot id, topic id, processor id) without
	// the kind prefix.
	ID        string
	X         float64
	Y         float64
	UpdatedAt time.Time
}

// NewChartPosition validates and constructs a ChartPosition.
func NewChartPosition(orgID, kind, id string, x, y float64, now time.Time) (ChartPosition, error) {
	if orgID == "" {
		return ChartPosition{}, errors.New("chart position orgID is empty")
	}
	if !ValidChartNodeKind(kind) {
		return ChartPosition{}, errors.New("chart position kind is invalid")
	}
	if id == "" {
		return ChartPosition{}, errors.New("chart position id is empty")
	}
	if now.IsZero() {
		return ChartPosition{}, errors.New("chart position timestamp is zero")
	}
	return ChartPosition{
		OrganizationID: orgID,
		Kind:           kind,
		ID:             id,
		X:              x,
		Y:              y,
		UpdatedAt:      now,
	}, nil
}
