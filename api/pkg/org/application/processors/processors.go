// Package processors is the application service that owns the Processor
// CRUD use cases — Create, Update, Delete, Preview — plus the two
// structural invariants a Processor carries: every output branch has a
// durable identity and its own event stream, and the processor graph
// must stay acyclic. Both the REST handlers and the MCP tools
// (create_processor, update_processor, delete_processor, list/get)
// delegate here so those invariants cannot drift between callers.
//
// Per CLAUDE.md helix-org philosophy this is the only place that does
// structural derivation (allocating a branch's identity and stream) — it
// is not workflow: the branch *is* part of what a Processor means,
// exactly as Role.Tools is a Worker's capability. The service does not
// orchestrate anything an agent should decide.
package processors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// ErrCycle signals that a Create/Update would close a cycle in the
// source graph. Adapters map it to 409 Conflict.
var ErrCycle = errors.New("processor would create a cycle in the source graph")

// Processors owns the processor-mutation use cases.
type Processors struct {
	procs       store.Processors
	triggers    store.Triggers
	attachments store.WorkerAttachments
	now         func() time.Time
	newID       func() string
}

// Deps are the constructor-injected collaborators.
type Deps struct {
	Processors  store.Processors
	Triggers    store.Triggers
	Attachments store.WorkerAttachments
	Now         func() time.Time
	NewID       func() string
}

// New constructs the Processors service.
func New(deps Deps) *Processors {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	newID := deps.NewID
	if newID == nil {
		newID = func() string { return "stub" }
	}
	return &Processors{procs: deps.Processors, triggers: deps.Triggers, attachments: deps.Attachments, now: now, newID: newID}
}

// OutputSpec describes one desired output branch at create/update time.
// ID is normally empty — the service allocates a durable `po-<id>` and
// the stream that branch writes to. A non-empty ID re-states an existing
// branch (the conversion and reconcilers do this). Match/Label are
// carried onto the resulting processor.Output (Match is meaningful only
// to the filter Kind).
type OutputSpec struct {
	// ID is the durable branch identity. Callers may omit it; the
	// application service allocates one when the branch is created.
	ID    string
	Label string
	Match string
	// ManagedFor tags an auto-managed route with the orgchart.NodeID it
	// serves (see processor.Output.ManagedFor). Empty for ordinary
	// human-authored outputs.
	ManagedFor string
}

// CreateParams describes a new Processor. ID is optional (minted
// `p-<id>` when empty). Outputs default to a single unconditional
// branch when none are given (the common transform case).
type CreateParams struct {
	ID          string
	Name        string
	InputSource eventsource.SourceRef
	Kind        processor.Kind
	Config      json.RawMessage
	// CreatedBy anchors the processor to a Worker on the chart, or carries
	// processor.SystemActor ("helix") when automation owns it — which is how
	// a processor is marked Automated (see processor.Processor.Automated).
	CreatedBy string
	Outputs   []OutputSpec
}

// Create validates the config, allocates each branch's identity and
// stream, cycle-checks the resulting graph, and persists the Processor.
func (s *Processors) Create(ctx context.Context, orgID string, p CreateParams) (processor.Processor, error) {
	id := processor.ProcessorID(strings.TrimSpace(p.ID))
	if id == "" {
		id = processor.ProcessorID("p-" + s.newID())
	}
	if p.Name == "" {
		return processor.Processor{}, fmt.Errorf("processor name is empty")
	}
	if err := s.validateSource(ctx, orgID, id, p.InputSource); err != nil {
		return processor.Processor{}, err
	}

	specs := p.Outputs
	if len(specs) == 0 {
		specs = []OutputSpec{{}} // single unconditional output (transform default)
	}
	outputs := make([]processor.Output, 0, len(specs))
	for _, spec := range specs {
		outputs = append(outputs, s.newOutput(spec))
	}

	proc, err := processor.NewProcessor(id, p.Name, p.InputSource, p.Kind, p.Config, outputs, p.CreatedBy, s.now(), orgID)
	if err != nil {
		return processor.Processor{}, err
	}
	if err := s.checkAcyclic(ctx, orgID, proc, ""); err != nil {
		return processor.Processor{}, err
	}
	if err := s.procs.Create(ctx, proc); err != nil {
		return processor.Processor{}, err
	}
	return proc, nil
}

// UpdateParams describes the mutable fields. Outputs are left as-is
// (changing the branch set goes through AddOutput / RemoveOutput).
// InputSource uses pointer semantics: nil leaves the input unchanged; a
// non-nil value sets it — including the zero SourceRef, which
// DISCONNECTS the processor (deleting its input edge on the chart),
// leaving it inert. A non-zero value re-points it at a different source
// (drag-to-wire), re-running the cycle check.
type UpdateParams struct {
	Name        string
	Kind        processor.Kind
	Config      json.RawMessage
	InputSource *eventsource.SourceRef
}

// Update rewrites name/kind/config (and optionally the input source) on
// an existing Processor, re-running validation and the cycle check.
func (s *Processors) Update(ctx context.Context, orgID string, id processor.ProcessorID, p UpdateParams) (processor.Processor, error) {
	existing, err := s.procs.Get(ctx, orgID, id)
	if err != nil {
		return processor.Processor{}, err
	}
	existing.Name = p.Name
	existing.Kind = p.Kind
	existing.Config = p.Config
	if p.InputSource != nil {
		existing.InputSource = *p.InputSource
	}
	if err := s.validateSource(ctx, orgID, id, existing.InputSource); err != nil {
		return processor.Processor{}, err
	}
	if err := existing.Validate(); err != nil {
		return processor.Processor{}, err
	}
	if err := s.checkAcyclic(ctx, orgID, existing, id); err != nil {
		return processor.Processor{}, err
	}
	if err := s.procs.Update(ctx, existing); err != nil {
		return processor.Processor{}, err
	}
	return existing, nil
}

// AddOutput appends one output branch ("route") to an existing
// Processor, allocating its durable identity and stream. It re-validates
// the Kind config against the new output set and re-runs the cycle check
// before persisting. Returns the resulting Output so callers can attach a
// Worker to it.
func (s *Processors) AddOutput(ctx context.Context, orgID string, id processor.ProcessorID, spec OutputSpec) (processor.Output, error) {
	existing, err := s.procs.Get(ctx, orgID, id)
	if err != nil {
		return processor.Output{}, err
	}
	out := s.newOutput(spec)
	existing.Outputs = append(existing.Outputs, out)
	if err := existing.Validate(); err != nil {
		return processor.Output{}, err
	}
	if err := s.checkAcyclic(ctx, orgID, existing, id); err != nil {
		return processor.Output{}, err
	}
	if err := s.procs.Update(ctx, existing); err != nil {
		return processor.Output{}, err
	}
	return out, nil
}

// newOutput allocates a branch's durable identity and the stream it
// writes to. Both are minted here and never change afterwards: the id is
// what attachments and downstream inputs name, and the stream is what
// carries the branch's history.
func (s *Processors) newOutput(spec OutputSpec) processor.Output {
	outputID := strings.TrimSpace(spec.ID)
	if outputID == "" {
		outputID = "po-" + s.newID()
	}
	return processor.Output{
		ID:         outputID,
		StreamID:   streaming.StreamID("s-" + s.newID()),
		Match:      spec.Match,
		Label:      spec.Label,
		ManagedFor: spec.ManagedFor,
	}
}

// validateSource rejects an input that names a source that does not
// exist. A zero source (unwired) is allowed. A Processor naming one of
// its own branches is a cycle, not a missing row — reported as ErrCycle
// so the adapter maps it to 409 like every other cycle.
func (s *Processors) validateSource(ctx context.Context, orgID string, id processor.ProcessorID, src eventsource.SourceRef) error {
	if src.Zero() {
		return nil
	}
	if err := src.Validate(); err != nil {
		return fmt.Errorf("validate processor input: %w", err)
	}
	if src.Kind == eventsource.KindProcessorOutput && src.ProcessorID == string(id) {
		return fmt.Errorf("%w: input %q is this processor's own branch", ErrCycle, src.Key())
	}
	switch src.Kind {
	case eventsource.KindTrigger:
		if s.triggers == nil {
			return errors.New("validate processor input: trigger repository is not configured")
		}
		rows, err := s.triggers.Find(ctx, store.WithOrg(orgID), store.WithID(src.TriggerID), store.WithLimit(1))
		if err != nil {
			return fmt.Errorf("validate processor input trigger %q: %w", src.TriggerID, err)
		}
		if len(rows) == 0 {
			return fmt.Errorf("validate processor input trigger %q: %w", src.TriggerID, store.ErrNotFound)
		}
	case eventsource.KindProcessorOutput:
		upstream, err := s.procs.Get(ctx, orgID, src.ProcessorID)
		if err != nil {
			return fmt.Errorf("validate processor input %q: %w", src.Key(), err)
		}
		if _, ok := upstream.Output(src.OutputID); !ok {
			return fmt.Errorf("validate processor input %q: %w", src.Key(), store.ErrNotFound)
		}
	}
	return nil
}

// RemoveOutput drops the branch with the given durable id. Idempotent:
// an unknown id is a no-op. Rejected when the branch still has Worker
// attachments, or when removing it would leave the Processor with zero
// outputs (Validate forbids that) — delete the whole Processor instead.
func (s *Processors) RemoveOutput(ctx context.Context, orgID string, id processor.ProcessorID, outputID string) error {
	existing, err := s.procs.Get(ctx, orgID, id)
	if err != nil {
		return err
	}
	idx := -1
	for i, o := range existing.Outputs {
		if o.ID == outputID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil // already gone
	}
	removed := existing.Outputs[idx]
	if s.attachments == nil {
		return errors.New("remove processor output: attachment repository is not configured")
	}
	attached, err := s.attachments.Find(ctx, store.WithOrg(orgID), store.WithProcessorID(id), store.WithOutputID(removed.ID), store.WithLimit(1))
	if err != nil {
		return fmt.Errorf("check output %q attachments: %w", removed.ID, err)
	}
	if len(attached) > 0 {
		return fmt.Errorf("processor output %q has worker attachments: %w", removed.ID, store.ErrConflict)
	}
	existing.Outputs = append(existing.Outputs[:idx:idx], existing.Outputs[idx+1:]...)
	if err := existing.Validate(); err != nil {
		return fmt.Errorf("remove output %q: %w", outputID, err)
	}
	return s.procs.Update(ctx, existing)
}

// Delete removes the Processor and every Worker attachment to its
// outputs. The branches' event history is retained: nothing else
// addresses those streams, and dropping the record of what a Worker was
// woken by is not this delete's job.
func (s *Processors) Delete(ctx context.Context, orgID string, id processor.ProcessorID) error {
	if s.attachments == nil {
		return errors.New("delete processor: attachment repository is not configured")
	}
	if _, err := s.procs.Get(ctx, orgID, id); err != nil {
		return err
	}
	attached, err := s.attachments.Find(ctx, store.WithOrg(orgID), store.WithProcessorID(id))
	if err != nil {
		return fmt.Errorf("delete processor: list attachments: %w", err)
	}
	if err := s.procs.Delete(ctx, orgID, id); err != nil {
		return err
	}
	for _, a := range attached {
		if err := s.attachments.Delete(ctx, orgID, a.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("processor deleted but attachment %q cleanup failed: %w", a.ID, err)
		}
	}
	return nil
}

// DeleteAutomatedByInput deletes every Automated Processor reading the
// given source. It is how the Slack auto-router is torn down when its
// workspace Trigger — or the workspace integration itself — is deleted:
// the router's lifecycle is bound to the source it reads. Human-authored
// processors reading the same source are left untouched (they become
// inert, but the operator owns them). Idempotent.
func (s *Processors) DeleteAutomatedByInput(ctx context.Context, orgID string, src eventsource.SourceRef) error {
	all, err := s.procs.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list processors: %w", err)
	}
	for _, p := range all {
		if p.InputSource != src || !p.Automated() {
			continue
		}
		if err := s.Delete(ctx, orgID, p.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("delete automated processor %q: %w", p.ID, err)
		}
	}
	return nil
}

// Get returns one Processor.
func (s *Processors) Get(ctx context.Context, orgID string, id processor.ProcessorID) (processor.Processor, error) {
	return s.procs.Get(ctx, orgID, id)
}

// List returns every Processor in the org.
func (s *Processors) List(ctx context.Context, orgID string) ([]processor.Processor, error) {
	return s.procs.List(ctx, orgID)
}

// checkAcyclic rejects a candidate Processor that would close a cycle in
// the source graph: starting from any of the candidate's output branches
// and following processor edges (input→outputs), the candidate's input
// must not be reachable. excludeID (non-empty on Update) drops the
// pre-update version of the candidate from the graph so re-saving an
// unchanged processor is never seen as its own cycle.
func (s *Processors) checkAcyclic(ctx context.Context, orgID string, candidate processor.Processor, excludeID processor.ProcessorID) error {
	if candidate.InputSource.Zero() {
		return nil // unwired: nothing to loop back to
	}
	all, err := s.procs.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("cycle check: list processors: %w", err)
	}
	// Adjacency: source key -> source keys reachable in one processor hop.
	edges := map[string][]string{}
	add := func(p processor.Processor) {
		if p.InputSource.Zero() {
			return
		}
		for _, o := range p.Outputs {
			edges[p.InputSource.Key()] = append(edges[p.InputSource.Key()], p.Source(o).Key())
		}
	}
	for _, p := range all {
		if p.ID == candidate.ID || (excludeID != "" && p.ID == excludeID) {
			continue
		}
		add(p)
	}
	add(candidate)

	// DFS from each candidate branch; a path back to the input is a cycle.
	target := candidate.InputSource.Key()
	seen := map[string]bool{}
	var reaches func(key string) bool
	reaches = func(key string) bool {
		if key == target {
			return true
		}
		if seen[key] {
			return false
		}
		seen[key] = true
		for _, next := range edges[key] {
			if reaches(next) {
				return true
			}
		}
		return false
	}
	for _, o := range candidate.Outputs {
		if reaches(candidate.Source(o).Key()) {
			return fmt.Errorf("%w: output %q leads back to input %q", ErrCycle, o.ID, target)
		}
	}
	return nil
}
