// Package bots is the application service that owns the structural Bot
// use cases — Create, Update, the reporting-line edges (AddParent /
// RemoveParent), and the base-tool Reconcile backfill. It is the single
// home for the bot-mutation logic the MCP tools and REST handlers drive,
// so the semantics cannot drift between callers.
//
// It is the merge of the former `roles` and `workers` application
// services: now that a Bot IS its own job description (the former Role
// and Worker collapsed into one aggregate), "edit a bot's content/tools"
// and "wire a bot's reporting lines" are operations on the same entity.
//
// Create/Update do a proper read-modify-write that preserves unpatched
// fields (a content-only update keeps Tools/Topics). The service depends
// only on the narrow store.Bots + store.ReportingLines repositories, the
// reconciler, a clock, an id-generator, and the injected base-tool list
// — never the whole *store.Store (CLAUDE.md: small interfaces). BaseTools
// is injected (rather than imported from the tools package) to keep the
// dependency edge one-way.
package bots

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// ErrReportingCycle is returned by AddParent when the proposed edge
// would close a loop in the reporting DAG. Adapters map it to 409.
var ErrReportingCycle = errors.New("reporting cycle")

// ErrReportingLinesUnavailable is returned when the reporting-lines
// repository is not wired. Adapters map it to 501.
var ErrReportingLinesUnavailable = errors.New("reporting lines not wired")

// Bots owns the bot-mutation use cases.
type Bots struct {
	bots       store.Bots
	lines      store.ReportingLines
	reconciler *reconcile.Reconciler
	now        func() time.Time
	newID      func() string
	baseTools  []tool.Name
}

// Deps are the constructor-injected collaborators for New.
type Deps struct {
	Bots store.Bots
	// Lines + Reconciler back AddParent/RemoveParent. Lines may be nil
	// (AddParent/RemoveParent then return ErrReportingLinesUnavailable);
	// Reconciler may be nil (no-op reconcile, handled by the Reconciler
	// itself).
	Lines      store.ReportingLines
	Reconciler *reconcile.Reconciler
	Now        func() time.Time
	NewID      func() string
	// BaseTools is the universal read baseline unioned into every
	// created Bot so no Bot can miss the read primitives every Bot
	// needs. Injected by the wiring (tools.BaseReadTools) to avoid an
	// import cycle.
	BaseTools []tool.Name
}

// New constructs the Bots service.
func New(deps Deps) *Bots {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Bots{
		bots:       deps.Bots,
		lines:      deps.Lines,
		reconciler: deps.Reconciler,
		now:        now,
		newID:      deps.NewID,
		baseTools:  deps.BaseTools,
	}
}

// CreateParams describes a new Bot. ID is optional — when empty a fresh
// `b-<id>` is minted. Tools is unioned with the injected base read
// tools; Topics is the typed manifest stored verbatim.
type CreateParams struct {
	ID      string
	Content string
	Tools   []tool.Name
	Topics  []streaming.TopicID
}

// Create builds and persists a new Bot, returning the created
// aggregate. The caller's tools are unioned with the base read tools
// (caller order preserved, baseline appended, deduped).
func (s *Bots) Create(ctx context.Context, orgID string, p CreateParams) (orgchart.Bot, error) {
	id := orgchart.BotID(strings.TrimSpace(p.ID))
	if id == "" {
		id = orgchart.BotID("b-" + s.newID())
	}
	bot, err := orgchart.NewBot(id, p.Content, MergeTools(p.Tools, s.baseTools), p.Topics, s.now(), orgID)
	if err != nil {
		return orgchart.Bot{}, err
	}
	if err := s.bots.Create(ctx, bot); err != nil {
		return orgchart.Bot{}, err
	}
	return bot, nil
}

// UpdateParams patches the mutable fields of a Bot. A nil pointer
// leaves the corresponding field unchanged — this is what preserves
// Tools/Topics on a content-only update.
type UpdateParams struct {
	Content *string
	Tools   *[]tool.Name
	Topics  *[]streaming.TopicID
}

// Update reads the existing Bot, applies the patch via the domain's
// With* builders, bumps UpdatedAt, and persists. Returns
// store.ErrNotFound (wrapped) when the (orgID, id) row is absent.
func (s *Bots) Update(ctx context.Context, orgID string, id orgchart.BotID, p UpdateParams) (orgchart.Bot, error) {
	existing, err := s.bots.Get(ctx, orgID, id)
	if err != nil {
		return orgchart.Bot{}, err
	}
	updated := existing
	if p.Content != nil {
		updated = updated.WithContent(*p.Content)
	}
	if p.Tools != nil {
		updated = updated.WithTools(*p.Tools)
	}
	if p.Topics != nil {
		updated = updated.WithTopics(*p.Topics)
	}
	updated = updated.WithUpdatedAt(s.now())
	if err := s.bots.Update(ctx, updated); err != nil {
		return orgchart.Bot{}, err
	}
	return updated, nil
}

// AddParent wires a reporting line (reportID reports to managerID),
// guarding the DAG against cycles, then reconciles the activation/team
// Topics the new edge implies. Both endpoints must exist. Idempotent:
// re-adding an existing line is a no-op (the repo's Add is idempotent).
// Returns ErrReportingCycle (→409), ErrReportingLinesUnavailable (→501),
// or store.ErrNotFound (→404) for the adapter to map.
func (s *Bots) AddParent(ctx context.Context, orgID string, reportID, managerID orgchart.BotID) error {
	if s.lines == nil {
		return ErrReportingLinesUnavailable
	}
	if _, err := s.bots.Get(ctx, orgID, reportID); err != nil {
		return fmt.Errorf("get bot %s: %w", reportID, err)
	}
	if _, err := s.bots.Get(ctx, orgID, managerID); err != nil {
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
func (s *Bots) guardCycle(ctx context.Context, orgID string, reportID, managerID orgchart.BotID) error {
	lines, err := s.lines.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list reporting lines: %w", err)
	}
	managersOf := map[orgchart.BotID][]orgchart.BotID{}
	for _, l := range lines {
		managersOf[l.ReportID] = append(managersOf[l.ReportID], l.ManagerID)
	}
	seen := map[orgchart.BotID]bool{}
	queue := []orgchart.BotID{managerID}
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
func (s *Bots) RemoveParent(ctx context.Context, orgID string, reportID, managerID orgchart.BotID) error {
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
// BaseTools) onto every Bot in the org. Idempotent: a Bot already at the
// baseline is left untouched (no write, no UpdatedAt bump). Order is
// stable — caller tools first, baseline appended in BaseTools order —
// because it reuses the same MergeTools the create path does.
func (s *Bots) Reconcile(ctx context.Context, orgID string) error {
	if s == nil {
		return nil
	}
	all, err := s.bots.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list bots: %w", err)
	}
	now := s.now()
	for _, bot := range all {
		merged := MergeTools(bot.Tools, s.baseTools)
		if sameToolList(bot.Tools, merged) {
			continue
		}
		updated := bot.WithTools(merged).WithUpdatedAt(now)
		if err := s.bots.Update(ctx, updated); err != nil {
			return fmt.Errorf("update bot %q: %w", bot.ID, err)
		}
	}
	return nil
}

// sameToolList reports element-wise equality. MergeTools is order-stable
// when the input already contains the baseline, so an in-order compare
// detects "no drift" — avoiding a write (and UpdatedAt bump) on Bots
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
// It is the single dedup-union algorithm shared by bot creation and the
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
