// Package nodes is the application service that owns the structural Node
// use cases — Create, Update, the reporting-line edges (AddParent /
// RemoveParent), and the base-tool Reconcile backfill. It is the single
// home for the node-mutation logic the MCP tools and REST handlers drive,
// so the semantics cannot drift between callers.
//
// It is the merge of the former `roles` and `workers` application
// services: now that a Node IS its own job description (the former Role
// and Worker collapsed into one aggregate), content/tools and reporting
// lines are operations on the same entity.
//
// Create/Update do a proper read-modify-write that preserves unpatched
// fields (a content-only update keeps Tools/Topics). The service depends
// only on the narrow store.Nodes + store.ReportingLines repositories, the
// reconciler, a clock, an id-generator, and the injected base-tool list
// — never the whole *store.Store (CLAUDE.md: small interfaces). BaseTools
// is injected (rather than imported from the tools package) to keep the
// dependency edge one-way.
package nodes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// ErrReportingCycle is returned by AddParent when the proposed edge
// would close a loop in the reporting DAG. Adapters map it to 409.
var ErrReportingCycle = errors.New("reporting cycle")

// ErrReportingLinesUnavailable is returned when the reporting-lines
// repository is not wired. Adapters map it to 501.
var ErrReportingLinesUnavailable = errors.New("reporting lines not wired")

// Nodes owns the node-mutation use cases.
type Nodes struct {
	nodes          store.Nodes
	lines          store.ReportingLines
	reconciler     *reconcile.Reconciler
	now            func() time.Time
	newID          func() string
	baseTools      []tool.Name
	onToolsChanged func(context.Context, string)
}

// Deps are the constructor-injected collaborators for New.
type Deps struct {
	Nodes store.Nodes
	// Lines + Reconciler back AddParent/RemoveParent. Lines may be nil
	// (AddParent/RemoveParent then return ErrReportingLinesUnavailable);
	// Reconciler may be nil (no-op reconcile, handled by the Reconciler
	// itself).
	Lines      store.ReportingLines
	Reconciler *reconcile.Reconciler
	Now        func() time.Time
	NewID      func() string
	// BaseTools is the universal read baseline unioned into every
	// created Node so no Node can miss the read primitives every Node
	// needs. Injected by the wiring (tools.BaseReadTools) to avoid an
	// import cycle.
	BaseTools      []tool.Name
	OnToolsChanged func(context.Context, string)
}

// New constructs the Nodes service.
func New(deps Deps) *Nodes {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Nodes{
		nodes:          deps.Nodes,
		lines:          deps.Lines,
		reconciler:     deps.Reconciler,
		now:            now,
		newID:          deps.NewID,
		baseTools:      deps.BaseTools,
		onToolsChanged: deps.OnToolsChanged,
	}
}

// CreateParams describes a new Node. ID is optional - when empty a fresh
// `b-<id>` is minted. Tools is unioned with the injected base read
// tools. Subscriptions are not part of the node row - the lifecycle
// service creates them as (node, topic) rows from its own CreateParams.
type CreateParams struct {
	ID              string
	Name            string
	Content         string
	AgentID         string
	Tools           []tool.Name
	PreserveContext bool
	// Kind, HelixUserID, Identity create a human placeholder when Kind ==
	// orgchart.NodeKindHuman. A human gets no base tools (it never makes an
	// MCP request) and is never spawned.
	Kind        orgchart.NodeKind
	HelixUserID string
	Identity    map[string]string
}

// Create builds and persists a new Node, returning the created
// aggregate. The caller's tools are unioned with the base read tools
// (caller order preserved, baseline appended, deduped).
func (s *Nodes) Create(ctx context.Context, orgID string, p CreateParams) (orgchart.Node, error) {
	id := orgchart.NodeID(strings.TrimSpace(p.ID))
	if id == "" {
		id = orgchart.NodeID("b-" + s.newID())
	}
	// The id is used exactly as given. It is unique within the org (composite
	// (id, org) primary key), so a clash means the id is already taken — return
	// a clear error rather than silently mutating it. Deterministic-id callers
	// (seeds) treat this as "already exists" after a Get.
	if _, err := s.nodes.Get(ctx, orgID, id); err == nil {
		return orgchart.Node{}, fmt.Errorf("node id %q already exists in this org", id)
	} else if !errors.Is(err, store.ErrNotFound) {
		return orgchart.Node{}, fmt.Errorf("check node id %q: %w", id, err)
	}
	// A human placeholder gets no tools — it never makes an MCP request.
	// An agent gets the caller's tools unioned with the read baseline.
	tools := p.Tools
	if p.Kind != orgchart.NodeKindHuman {
		tools = MergeTools(p.Tools, s.baseTools)
	}
	node, err := orgchart.NewNode(id, p.Content, tools, s.now(), orgID)
	if err != nil {
		return orgchart.Node{}, err
	}
	if p.Name != "" {
		node = node.WithName(p.Name)
	}
	if p.AgentID != "" {
		node = node.WithAgentID(p.AgentID)
	}
	if p.PreserveContext {
		node = node.WithPreserveContext(true)
	}
	if p.Kind != "" {
		node = node.WithKind(p.Kind)
	}
	if p.HelixUserID != "" {
		node = node.WithHelixUserID(p.HelixUserID)
	}
	if len(p.Identity) > 0 {
		node = node.WithIdentity(p.Identity)
	}
	if err := s.nodes.Create(ctx, node); err != nil {
		return orgchart.Node{}, err
	}
	return node, nil
}

// UpdateParams patches the mutable fields of a Node. A nil pointer
// leaves the corresponding field unchanged — this is what preserves
// Tools on a content-only update.
type UpdateParams struct {
	AgentID         *string
	Name            *string
	Content         *string
	Tools           *[]tool.Name
	ProjectIDs      *[]string
	PreserveContext *bool
	// Identity, when non-nil, replaces the node's per-channel handle map
	// (human nodes only). nil leaves it unchanged.
	Identity *map[string]string
}

// Update reads the existing Node, applies the patch via the domain's
// With* builders, bumps UpdatedAt, and persists. Returns
// store.ErrNotFound (wrapped) when the (orgID, id) row is absent.
func (s *Nodes) Update(ctx context.Context, orgID string, id orgchart.NodeID, p UpdateParams) (orgchart.Node, error) {
	existing, err := s.nodes.Get(ctx, orgID, id)
	if err != nil {
		return orgchart.Node{}, err
	}
	updated := existing
	if p.AgentID != nil {
		updated = updated.WithAgentID(*p.AgentID)
	}
	if p.Name != nil {
		updated = updated.WithName(*p.Name)
	}
	if p.Content != nil {
		updated = updated.WithContent(*p.Content)
	}
	if p.Tools != nil {
		updated = updated.WithTools(*p.Tools)
	}
	if p.ProjectIDs != nil {
		updated = updated.WithProjectIDs(normalizeProjectIDs(*p.ProjectIDs))
	}
	if p.PreserveContext != nil {
		updated = updated.WithPreserveContext(*p.PreserveContext)
	}
	if p.Identity != nil {
		updated = updated.WithIdentity(*p.Identity)
	}
	updated = updated.WithUpdatedAt(s.now())
	if err := s.nodes.Update(ctx, updated); err != nil {
		return orgchart.Node{}, err
	}
	if p.Tools != nil {
		s.notifyToolsChanged(ctx, updated.AgentID)
	}
	return updated, nil
}

func normalizeProjectIDs(projectIDs []string) []string {
	seen := make(map[string]struct{}, len(projectIDs))
	out := make([]string, 0, len(projectIDs))
	for _, projectID := range projectIDs {
		projectID = strings.TrimSpace(projectID)
		if projectID == "" {
			continue
		}
		if _, ok := seen[projectID]; ok {
			continue
		}
		seen[projectID] = struct{}{}
		out = append(out, projectID)
	}
	return out
}

// AttachTools grants the named tools to a Node: the union of its current
// tools and names (caller order preserved, new names appended, deduped),
// persisted. Idempotent per name - names the Node already has are no-ops,
// and a call that adds nothing writes nothing. Returns store.ErrNotFound
// (wrapped) when the (orgID, id) row is absent.
func (s *Nodes) AttachTools(ctx context.Context, orgID string, id orgchart.NodeID, names []tool.Name) (orgchart.Node, error) {
	existing, err := s.nodes.Get(ctx, orgID, id)
	if err != nil {
		return orgchart.Node{}, err
	}
	merged := MergeTools(existing.Tools, names)
	if sameToolList(existing.Tools, merged) {
		return existing, nil
	}
	updated := existing.WithTools(merged).WithUpdatedAt(s.now())
	if err := s.nodes.Update(ctx, updated); err != nil {
		return orgchart.Node{}, err
	}
	s.notifyToolsChanged(ctx, updated.AgentID)
	return updated, nil
}

// DetachTools removes the named tools from a Node. Idempotent per name (a
// name the Node lacks is a no-op). It refuses to remove any universal
// read-baseline tool — those are mandatory and the reconciler would
// re-add them — failing the whole call before any write. Returns
// store.ErrNotFound (wrapped) when the (orgID, id) row is absent.
func (s *Nodes) DetachTools(ctx context.Context, orgID string, id orgchart.NodeID, names []tool.Name) (orgchart.Node, error) {
	base := make(map[tool.Name]struct{}, len(s.baseTools))
	for _, b := range s.baseTools {
		base[b] = struct{}{}
	}
	remove := make(map[tool.Name]struct{}, len(names))
	for _, n := range names {
		if _, ok := base[n]; ok {
			return orgchart.Node{}, fmt.Errorf("cannot detach baseline tool %q", n)
		}
		remove[n] = struct{}{}
	}
	existing, err := s.nodes.Get(ctx, orgID, id)
	if err != nil {
		return orgchart.Node{}, err
	}
	kept := make([]tool.Name, 0, len(existing.Tools))
	for _, t := range existing.Tools {
		if _, drop := remove[t]; drop {
			continue
		}
		kept = append(kept, t)
	}
	if len(kept) == len(existing.Tools) {
		return existing, nil
	}
	updated := existing.WithTools(kept).WithUpdatedAt(s.now())
	if err := s.nodes.Update(ctx, updated); err != nil {
		return orgchart.Node{}, err
	}
	s.notifyToolsChanged(ctx, updated.AgentID)
	return updated, nil
}

func (s *Nodes) notifyToolsChanged(ctx context.Context, appID string) {
	if s.onToolsChanged != nil && appID != "" {
		s.onToolsChanged(ctx, appID)
	}
}

// AddParent wires a reporting line (reportID reports to managerID),
// guarding the DAG against cycles, then reconciles the activation/team
// Topics the new edge implies. Both endpoints must exist. Idempotent:
// re-adding an existing line is a no-op (the repo's Add is idempotent).
// Returns ErrReportingCycle (→409), ErrReportingLinesUnavailable (→501),
// or store.ErrNotFound (→404) for the adapter to map.
func (s *Nodes) AddParent(ctx context.Context, orgID string, reportID, managerID orgchart.NodeID) error {
	if s.lines == nil {
		return ErrReportingLinesUnavailable
	}
	if _, err := s.nodes.Get(ctx, orgID, reportID); err != nil {
		return fmt.Errorf("get node %s: %w", reportID, err)
	}
	if _, err := s.nodes.Get(ctx, orgID, managerID); err != nil {
		return fmt.Errorf("get manager %s: %w", managerID, err)
	}
	line, err := orgchart.NewReportingLine(orgID, managerID, reportID)
	if err != nil {
		return err
	}
	if err := s.guardCycle(ctx, orgID, reportID, managerID); err != nil {
		return err
	}
	if err := s.lines.Add(ctx, line); err != nil {
		return fmt.Errorf("add reporting line: %w", err)
	}
	// Pass both endpoints so the manager's team topic is in scope.
	if err := s.reconciler.Reconcile(ctx, orgID, reportID, managerID); err != nil {
		return fmt.Errorf("reconcile topology: %w", err)
	}
	return nil
}

// guardCycle walks up the DAG from managerID; if reportID is reachable,
// adding (manager → report) would close a loop.
func (s *Nodes) guardCycle(ctx context.Context, orgID string, reportID, managerID orgchart.NodeID) error {
	lines, err := s.lines.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list reporting lines: %w", err)
	}
	managersOf := map[orgchart.NodeID][]orgchart.NodeID{}
	for _, l := range lines {
		managersOf[l.ReportID] = append(managersOf[l.ReportID], l.ManagerID)
	}
	seen := map[orgchart.NodeID]bool{}
	queue := []orgchart.NodeID{managerID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == reportID {
			return fmt.Errorf("making %s report to %s would create a reporting cycle: %w", reportID, managerID, ErrReportingCycle)
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		queue = append(queue, managersOf[cur]...)
	}
	return nil
}

// RemoveParent drops the (reportID → managerID) reporting line, then
// reconciles the Topics the dropped edge implies. Returns
// ErrReportingLinesUnavailable (→501) or store.ErrNotFound (→404).
func (s *Nodes) RemoveParent(ctx context.Context, orgID string, reportID, managerID orgchart.NodeID) error {
	if s.lines == nil {
		return ErrReportingLinesUnavailable
	}
	if err := s.lines.Remove(ctx, orgID, reportID, managerID); err != nil {
		return fmt.Errorf("remove reporting line %s→%s: %w", reportID, managerID, err)
	}
	// Both endpoints named — the ex-manager is no longer in
	// ListManagers(report), so it must be explicit to fall in scope.
	if err := s.reconciler.Reconcile(ctx, orgID, reportID, managerID); err != nil {
		return fmt.Errorf("reconcile topology: %w", err)
	}
	return nil
}

// Reconcile backfills the universal read baseline (the injected
// BaseTools) onto every Node in the org. Idempotent: a Node already at the
// baseline is left untouched (no write, no UpdatedAt bump). Order is
// stable — caller tools first, baseline appended in BaseTools order —
// because it reuses the same MergeTools the create path does.
func (s *Nodes) Reconcile(ctx context.Context, orgID string) error {
	if s == nil {
		return nil
	}
	all, err := s.nodes.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	now := s.now()
	for _, node := range all {
		merged := MergeTools(node.Tools, s.baseTools)
		if sameToolList(node.Tools, merged) {
			continue
		}
		updated := node.WithTools(merged).WithUpdatedAt(now)
		if err := s.nodes.Update(ctx, updated); err != nil {
			return fmt.Errorf("update node %q: %w", node.ID, err)
		}
		s.notifyToolsChanged(ctx, updated.AgentID)
	}
	return nil
}

// sameToolList reports element-wise equality. MergeTools is order-stable
// when the input already contains the baseline, so an in-order compare
// detects "no drift" — avoiding a write (and UpdatedAt bump) on Nodes
// already at the baseline.
func sameToolList(a, b []tool.Name) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// MergeTools returns the union of `existing` and `base`: the order of
// `existing` is preserved, any `base` entries not already present are
// appended in base order, and duplicates within `existing` are dropped.
// It is the single dedup-union algorithm shared by node creation and the
// tools-package reconciler.
func MergeTools(existing, base []tool.Name) []tool.Name {
	seen := make(map[tool.Name]struct{}, len(existing)+len(base))
	out := make([]tool.Name, 0, len(existing)+len(base))
	for _, name := range existing {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, name := range base {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
