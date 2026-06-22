// Package processors is the application service that owns the Processor
// CRUD use cases — Create, Update, Delete, Preview — plus the two
// structural invariants a Processor carries: its output Topics are
// auto-provisioned and owned by it, and the processor graph must stay
// acyclic. Both the REST handlers and (later) any MCP tool delegate
// here so those invariants cannot drift between callers.
//
// Per CLAUDE.md helix-org philosophy this is the only place that does
// structural derivation (auto-creating output topics) — it is not
// workflow: the output topic *is* part of what a Processor means,
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

	"github.com/helixml/helix/api/pkg/org/application/topics"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// ErrCycle signals that a Create/Update would close a cycle in the
// topic graph. Adapters map it to 409 Conflict.
var ErrCycle = errors.New("processor would create a cycle in the topic graph")

// TopicWriter is the narrow slice of the topics application service the
// processors service needs to auto-provision and tear down output
// Topics. *topics.Topics satisfies it.
type TopicWriter interface {
	Create(ctx context.Context, orgID string, p topics.CreateParams) (streaming.Topic, error)
	Delete(ctx context.Context, orgID string, id streaming.TopicID) error
}

// Processors owns the processor-mutation use cases.
type Processors struct {
	procs  store.Processors
	topics TopicWriter
	now    func() time.Time
	newID  func() string
}

// Deps are the constructor-injected collaborators.
type Deps struct {
	Processors store.Processors
	Topics     TopicWriter
	Now        func() time.Time
	NewID      func() string
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
	return &Processors{procs: deps.Processors, topics: deps.Topics, now: now, newID: newID}
}

// OutputSpec describes one desired output branch at create/update time.
// TopicID is normally empty — the service auto-provisions a local Topic
// and fills it in (design decision 2). A non-empty TopicID points the
// branch at an existing Topic instead of provisioning one (OQ1's
// "publish into an existing topic"): it enables fan-in to a shared
// topic, and it is the only way to form a graph the cycle check must
// reject — so it is what makes checkAcyclic a live guard rather than
// dead code. Explicit-output topics are not owned by the processor and
// are left intact on delete. Match/Label are carried onto the resulting
// processor.Output (Match is meaningful only to the filter Kind).
type OutputSpec struct {
	TopicID streaming.TopicID
	Label   string
	Match   string
}

// CreateParams describes a new Processor. ID is optional (minted
// `p-<id>` when empty). Outputs default to a single unconditional
// branch when none are given (the common transform case).
type CreateParams struct {
	ID           string
	Name         string
	InputTopicID streaming.TopicID
	Kind         processor.Kind
	Config       json.RawMessage
	CreatedBy    string
	Outputs      []OutputSpec
}

// Create validates the config, auto-provisions the output Topic(s),
// cycle-checks the resulting graph, and persists the Processor. On a
// cycle or a persist failure it deletes any Topics it provisioned so a
// failed create leaves no orphans.
func (s *Processors) Create(ctx context.Context, orgID string, p CreateParams) (processor.Processor, error) {
	id := processor.ProcessorID(strings.TrimSpace(p.ID))
	if id == "" {
		id = processor.ProcessorID("p-" + s.newID())
	}
	if p.Name == "" {
		return processor.Processor{}, fmt.Errorf("processor name is empty")
	}

	specs := p.Outputs
	if len(specs) == 0 {
		specs = []OutputSpec{{}} // single unconditional output (transform default)
	}

	outputs := make([]processor.Output, 0, len(specs))
	provisioned := make([]streaming.TopicID, 0, len(specs))
	rollback := func() {
		for _, tid := range provisioned {
			_ = s.topics.Delete(ctx, orgID, tid)
		}
	}

	for i, spec := range specs {
		if spec.TopicID != "" {
			// Explicit existing topic — not owned by the processor, so it
			// is not provisioned here and not torn down on delete.
			outputs = append(outputs, processor.Output{TopicID: spec.TopicID, Match: spec.Match, Label: spec.Label, Owned: false})
			continue
		}
		t, err := s.topics.Create(ctx, orgID, topics.CreateParams{
			Name:        outputTopicName(id, spec.Label, i, len(specs)),
			Description: fmt.Sprintf("Output of processor %s (%s)", id, p.Name),
			CreatedBy:   p.CreatedBy,
		})
		if err != nil {
			rollback()
			return processor.Processor{}, fmt.Errorf("provision output topic: %w", err)
		}
		provisioned = append(provisioned, t.ID)
		outputs = append(outputs, processor.Output{TopicID: t.ID, Match: spec.Match, Label: spec.Label, Owned: true})
	}

	proc, err := processor.NewProcessor(id, p.Name, p.InputTopicID, p.Kind, p.Config, outputs, p.CreatedBy, s.now(), orgID)
	if err != nil {
		rollback()
		return processor.Processor{}, err
	}
	if err := s.checkAcyclic(ctx, orgID, proc, ""); err != nil {
		rollback()
		return processor.Processor{}, err
	}
	if err := s.procs.Create(ctx, proc); err != nil {
		rollback()
		return processor.Processor{}, err
	}
	return proc, nil
}

// UpdateParams describes the mutable fields. Outputs are left as-is when
// nil; the auto-provisioned output Topics are not re-created on update
// (changing the branch count is a delete-and-recreate concern for v1).
// InputTopicID uses pointer semantics: nil leaves the input unchanged; a
// non-nil value sets it — including the empty string, which DISCONNECTS
// the processor (deleting its input edge on the chart), leaving it inert.
// A non-empty value re-points it at a different input (drag-to-wire),
// re-running the cycle check.
type UpdateParams struct {
	Name         string
	Kind         processor.Kind
	Config       json.RawMessage
	InputTopicID *streaming.TopicID
}

// Update rewrites name/kind/config (and optionally the input topic) on an
// existing Processor, re-running validation and the cycle check. Outputs
// are immutable here (v1).
func (s *Processors) Update(ctx context.Context, orgID string, id processor.ProcessorID, p UpdateParams) (processor.Processor, error) {
	existing, err := s.procs.Get(ctx, orgID, id)
	if err != nil {
		return processor.Processor{}, err
	}
	existing.Name = p.Name
	existing.Kind = p.Kind
	existing.Config = p.Config
	if p.InputTopicID != nil {
		existing.InputTopicID = *p.InputTopicID
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

// Delete removes the Processor and the output Topics it owns (cascading
// their subscriptions, as topic delete already does).
func (s *Processors) Delete(ctx context.Context, orgID string, id processor.ProcessorID) error {
	existing, err := s.procs.Get(ctx, orgID, id)
	if err != nil {
		return err
	}
	if err := s.procs.Delete(ctx, orgID, id); err != nil {
		return err
	}
	for _, o := range existing.Outputs {
		if !o.Owned {
			continue // explicit/shared topic — outlives the processor
		}
		if err := s.topics.Delete(ctx, orgID, o.TopicID); err != nil {
			// Idempotent: an already-deleted output topic (ErrNotFound) is
			// fine — the goal state is reached. Any other error is logged
			// via the wrap but doesn't fail the delete (the processor row
			// is already gone, so a leftover output topic is inert).
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return fmt.Errorf("processor deleted but output topic %q cleanup failed: %w", o.TopicID, err)
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

// OwnerOfOutput returns the id of the Processor that owns topicID as an
// auto-provisioned output, and true, if any does. The topic-delete
// handler uses it to block independent deletion of a processor-managed
// output topic (which would leave the processor with a dangling output);
// the caller should delete the processor instead, which cascades it.
func (s *Processors) OwnerOfOutput(ctx context.Context, orgID string, topicID streaming.TopicID) (processor.ProcessorID, bool, error) {
	all, err := s.procs.List(ctx, orgID)
	if err != nil {
		return "", false, err
	}
	for _, p := range all {
		for _, o := range p.Outputs {
			if o.Owned && o.TopicID == topicID {
				return p.ID, true, nil
			}
		}
	}
	return "", false, nil
}

// checkAcyclic rejects a candidate Processor that would close a cycle in
// the topic graph: starting from any of the candidate's output topics
// and following processor edges (input→outputs), the candidate's input
// topic must not be reachable. excludeID (non-empty on Update) drops the
// pre-update version of the candidate from the graph so re-saving an
// unchanged processor is never seen as its own cycle.
func (s *Processors) checkAcyclic(ctx context.Context, orgID string, candidate processor.Processor, excludeID processor.ProcessorID) error {
	all, err := s.procs.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("cycle check: list processors: %w", err)
	}
	// Adjacency: topic -> topics reachable in one processor hop.
	edges := map[streaming.TopicID][]streaming.TopicID{}
	add := func(p processor.Processor) {
		for _, o := range p.Outputs {
			edges[p.InputTopicID] = append(edges[p.InputTopicID], o.TopicID)
		}
	}
	for _, p := range all {
		if p.ID == candidate.ID || (excludeID != "" && p.ID == excludeID) {
			continue
		}
		add(p)
	}
	add(candidate)

	// DFS from each candidate output; a path back to the input is a cycle.
	target := candidate.InputTopicID
	seen := map[streaming.TopicID]bool{}
	var reaches func(t streaming.TopicID) bool
	reaches = func(t streaming.TopicID) bool {
		if t == target {
			return true
		}
		if seen[t] {
			return false
		}
		seen[t] = true
		for _, next := range edges[t] {
			if reaches(next) {
				return true
			}
		}
		return false
	}
	for _, o := range candidate.Outputs {
		if reaches(o.TopicID) {
			return fmt.Errorf("%w: output %q leads back to input %q", ErrCycle, o.TopicID, candidate.InputTopicID)
		}
	}
	return nil
}

// outputTopicName builds a unique, human-readable name for an
// auto-provisioned output topic. Processor names are unique per org, so
// `<procID> output` (single) / `<procID> · <label>` (multi) cannot
// collide across processors; the topic id itself is independently
// minted and unique.
func outputTopicName(id processor.ProcessorID, label string, idx, total int) string {
	if total <= 1 {
		return fmt.Sprintf("%s output", id)
	}
	if label == "" {
		label = fmt.Sprintf("out-%d", idx)
	}
	return fmt.Sprintf("%s · %s", id, label)
}
