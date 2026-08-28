// Package cutover converts the retired Topic model into the Trigger
// model exactly once, in place, over the repository interfaces.
//
// It is deliberately not a schema migration: there is no migration
// table, no checkpoint, no cutover flag and no dual write. Convert is
// repeat-safe by construction — a target row that already exists with
// the expected value is success, and a conflicting one is an error, so
// running it on every boot converges and then does nothing.
//
// Repeat-safe is not the same as run-once, and the difference is the
// user's deletes. A retired row that survives its own conversion is a
// standing instruction to recreate that Trigger or attachment: delete
// the Trigger, redeploy, and the next boot builds it again, because
// "does the target exist?" cannot tell a not-yet-converted row from a
// converted-then-deleted one. So the conversion CONSUMES what it
// reads — each retired row is deleted once it has been handled, which
// is the tombstone. "Handled" includes the outcomes that create
// nothing: a Processor output Topic (skipped by design), a dangling or
// human subscription, and a repeat run finding the target already
// there. A skipped output Topic in particular must not be left behind,
// because deleting its Processor removes the branch that identifies it
// and the row would then convert into a Trigger for a stream nobody
// owns.
//
// Consuming a row is destructive by design: after conversion the
// retired tables are dead weight awaiting a DROP, and the Processor
// input column is already cleared the same way.
//
// What it converts, and the invariant each conversion preserves:
//
//   - Every workflow Topic becomes a Trigger with the SAME id. That is
//     the history invariant: a Trigger's event stream is its own id, so
//     every persisted event stays addressable and no event row is copied
//     or rewritten.
//   - A Processor-owned output Topic becomes nothing: the branch already
//     records that Topic as its stream, so the events stay where they
//     are and the branch's durable id replaces the Topic as the thing
//     Workers address.
//   - Every subscription becomes a Worker attachment — to the same-id
//     Trigger, or, when the Topic was a Processor output, to that
//     Processor's exact branch.
//   - Every Processor's input Topic becomes a terminal source reference.
//
// Outbound-only Topic configuration is deliberately discarded: after the
// outbound-actions work a Worker acts on a provider with its own
// credential and that provider's native API, so there is nothing for a
// Trigger to carry.
package cutover

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/attachment"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Deps are the repositories the conversion reads and writes.
type Deps struct {
	Store  *store.Store
	Logger *slog.Logger
}

// Result is what one Convert run did. Every count is zero on a
// converged deployment, which is what makes "did this run do anything?"
// answerable from the logs.
type Result struct {
	Triggers    int
	Attachments int
	Inputs      int
	Skipped     int
}

// Convert runs the whole conversion across every organization. It is
// safe to call on every boot and on a deployment that never ran a
// pre-cutover release (there is simply nothing to read).
func Convert(ctx context.Context, d Deps) (Result, error) {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	var result Result
	if d.Store == nil || d.Store.RetiredTopics == nil {
		return result, nil
	}

	topics, err := d.Store.RetiredTopics.ListAll(ctx)
	if err != nil {
		return result, fmt.Errorf("cutover: read retired topics: %w", err)
	}
	subscriptions, err := d.Store.RetiredSubscriptions.ListAll(ctx)
	if err != nil {
		return result, fmt.Errorf("cutover: read retired subscriptions: %w", err)
	}
	legacyInputs, err := d.Store.RetiredProcessorInputs.ListAll(ctx)
	if err != nil {
		return result, fmt.Errorf("cutover: read retired processor inputs: %w", err)
	}
	if len(topics) == 0 && len(subscriptions) == 0 && len(legacyInputs) == 0 {
		return result, nil
	}

	// Index every Processor branch by the stream it writes, per org.
	// That map is what tells a Topic apart from a Processor output: a
	// Topic a branch writes to is not an inbound source and must not
	// become a Trigger.
	branches, err := branchesByStream(ctx, d.Store, orgIDsOf(topics, subscriptions, legacyInputs))
	if err != nil {
		return result, err
	}

	for _, topic := range topics {
		key := store.OrgScopedID{OrgID: topic.OrganizationID, ID: topic.ID}
		if _, isBranch := branches[key]; isBranch {
			result.Skipped++
		} else {
			created, err := convertTopic(ctx, d.Store, topic)
			if err != nil {
				return result, err
			}
			if created {
				result.Triggers++
			}
		}
		if err := d.Store.RetiredTopics.Delete(ctx, topic.OrganizationID, topic.ID); err != nil {
			return result, fmt.Errorf("cutover: consume retired topic %q: %w", topic.ID, err)
		}
	}

	for _, sub := range subscriptions {
		created, err := convertSubscription(ctx, d.Store, sub, branches)
		if err != nil {
			return result, err
		}
		if created {
			result.Attachments++
		}
		if err := d.Store.RetiredSubscriptions.Delete(ctx, sub.OrganizationID, sub.NodeID, string(sub.TopicID)); err != nil {
			return result, fmt.Errorf("cutover: consume retired subscription %q→%q: %w", sub.NodeID, sub.TopicID, err)
		}
	}

	for key, inputTopic := range legacyInputs {
		converted, err := convertProcessorInput(ctx, d, key, inputTopic, branches)
		if err != nil {
			return result, err
		}
		if converted {
			result.Inputs++
		}
	}

	logger.Info("cutover: converted retired topic model",
		"triggers", result.Triggers,
		"attachments", result.Attachments,
		"processor_inputs", result.Inputs,
		"processor_output_topics_skipped", result.Skipped)
	return result, nil
}

// convertTopic creates the same-id Trigger for one Topic. Returns false
// when the Trigger already exists (a repeat run).
func convertTopic(ctx context.Context, s *store.Store, topic streaming.Topic) (bool, error) {
	existing, err := s.Triggers.Find(ctx, store.WithOrg(topic.OrganizationID), store.WithID(topic.ID), store.WithLimit(1))
	if err != nil {
		return false, fmt.Errorf("cutover: look up trigger %q: %w", topic.ID, err)
	}
	if len(existing) > 0 {
		return false, nil
	}
	converted, err := TopicToTrigger(topic)
	if err != nil {
		return false, fmt.Errorf("cutover: %w", err)
	}
	if err := s.Triggers.Create(ctx, converted); err != nil {
		// A name clash (rather than an id clash) means a Trigger with
		// this name was created independently before the conversion ran.
		// That is a genuine conflict a human must resolve — the two rows
		// describe different sources under one name.
		return false, fmt.Errorf("cutover: create trigger %q from topic: %w", topic.ID, err)
	}
	return true, nil
}

// convertSubscription creates the Worker attachment one subscription
// implies. Returns false when the attachment already exists, or when the
// subscription is unconvertible for a reason that is not a failure:
//   - the Worker row is gone (a dangling subscription the cascade
//     never reached);
//   - the Worker is human — attachments are AI-Worker-only, and a human
//     never had an activation to receive.
func convertSubscription(ctx context.Context, s *store.Store, sub streaming.Subscription, branches map[store.OrgScopedID]branchRef) (bool, error) {
	workerID := orgchart.NodeID(sub.NodeID)
	node, err := s.Nodes.Get(ctx, sub.OrganizationID, workerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("cutover: look up worker %q: %w", workerID, err)
	}
	if node.IsHuman() {
		return false, nil
	}

	src := eventsource.Trigger(sub.TopicID)
	if branch, ok := branches[store.OrgScopedID{OrgID: sub.OrganizationID, ID: sub.TopicID}]; ok {
		src = eventsource.ProcessorOutput(branch.ProcessorID, branch.OutputID)
	}

	opts := []store.Option{store.WithOrg(sub.OrganizationID), store.WithWorkerID(workerID)}
	if src.Kind == eventsource.KindTrigger {
		opts = append(opts, store.WithTriggerID(src.TriggerID))
	} else {
		opts = append(opts, store.WithProcessorID(src.ProcessorID), store.WithOutputID(src.OutputID))
	}
	existing, err := s.WorkerAttachments.Find(ctx, append(opts, store.WithLimit(1))...)
	if err != nil {
		return false, fmt.Errorf("cutover: look up attachment %q→%s: %w", workerID, src.Key(), err)
	}
	if len(existing) > 0 {
		return false, nil
	}

	// A subscription to an ordinary Topic can only become an attachment
	// once that Topic's Trigger exists. If it does not, the Topic row is
	// gone and the subscription is dangling — same non-failure as a
	// missing Worker.
	if src.Kind == eventsource.KindTrigger {
		rows, err := s.Triggers.Find(ctx, store.WithOrg(sub.OrganizationID), store.WithID(src.TriggerID), store.WithLimit(1))
		if err != nil {
			return false, fmt.Errorf("cutover: look up trigger %q: %w", src.TriggerID, err)
		}
		if len(rows) == 0 {
			return false, nil
		}
	}

	a, err := attachment.New(convertedAttachmentID(workerID, src), sub.OrganizationID, workerID, src, string(node.ID), attachmentTime(sub.CreatedAt))
	if err != nil {
		return false, fmt.Errorf("cutover: build attachment %q→%s: %w", workerID, src.Key(), err)
	}
	if err := s.WorkerAttachments.Create(ctx, a); err != nil {
		return false, fmt.Errorf("cutover: create attachment %q→%s: %w", workerID, src.Key(), err)
	}
	return true, nil
}

// convertProcessorInput rewrites one Processor's retired input Topic as
// a terminal source reference and clears the retired column. Returns
// false when the Processor row is gone.
func convertProcessorInput(ctx context.Context, d Deps, key store.OrgScopedID, inputTopic streaming.StreamID, branches map[store.OrgScopedID]branchRef) (bool, error) {
	p, err := d.Store.Processors.Get(ctx, key.OrgID, key.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("cutover: look up processor %q: %w", key.ID, err)
	}
	if !p.InputSource.Zero() {
		// Already converted by an earlier run that failed before
		// clearing the column. Clear it now so this run converges.
		return false, d.clear(ctx, key)
	}

	src := eventsource.Trigger(inputTopic)
	if branch, ok := branches[store.OrgScopedID{OrgID: key.OrgID, ID: inputTopic}]; ok {
		src = eventsource.ProcessorOutput(branch.ProcessorID, branch.OutputID)
	}
	p.InputSource = src
	if err := p.Validate(); err != nil {
		return false, fmt.Errorf("cutover: processor %q input %s: %w", key.ID, src.Key(), err)
	}
	if err := d.Store.Processors.Update(ctx, p); err != nil {
		return false, fmt.Errorf("cutover: update processor %q input: %w", key.ID, err)
	}
	return true, d.clear(ctx, key)
}

func (d Deps) clear(ctx context.Context, key store.OrgScopedID) error {
	if d.Store.RetiredProcessorInputs == nil {
		return nil
	}
	if err := d.Store.RetiredProcessorInputs.Clear(ctx, key.OrgID, key.ID); err != nil {
		return fmt.Errorf("cutover: clear retired input on processor %q: %w", key.ID, err)
	}
	return nil
}

// branchRef names the Processor branch that owns one event stream.
type branchRef struct {
	ProcessorID string
	OutputID    string
}

// branchesByStream indexes every Processor branch by the stream it
// writes. A pre-cutover branch's stream is the output Topic it was
// provisioned with, so this is exactly the "which Topics are really
// Processor outputs?" question the conversion has to answer.
func branchesByStream(ctx context.Context, s *store.Store, orgIDs []string) (map[store.OrgScopedID]branchRef, error) {
	out := map[store.OrgScopedID]branchRef{}
	for _, orgID := range orgIDs {
		procs, err := s.Processors.List(ctx, orgID)
		if err != nil {
			return nil, fmt.Errorf("cutover: list processors in %q: %w", orgID, err)
		}
		for _, p := range procs {
			for _, o := range p.Outputs {
				out[store.OrgScopedID{OrgID: orgID, ID: o.StreamID}] = branchRef{ProcessorID: p.ID, OutputID: o.ID}
			}
		}
	}
	return out, nil
}

func orgIDsOf(topics []streaming.Topic, subs []streaming.Subscription, inputs map[store.OrgScopedID]streaming.StreamID) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(orgID string) {
		if orgID == "" {
			return
		}
		if _, ok := seen[orgID]; ok {
			return
		}
		seen[orgID] = struct{}{}
		out = append(out, orgID)
	}
	for _, t := range topics {
		add(t.OrganizationID)
	}
	for _, s := range subs {
		add(s.OrganizationID)
	}
	for key := range inputs {
		add(key.OrgID)
	}
	return out
}

// convertedAttachmentID derives the row id from the pair it represents,
// so a repeat run after a partial failure converges on the same row
// rather than accumulating duplicates.
func convertedAttachmentID(workerID orgchart.NodeID, src eventsource.SourceRef) string {
	return "wa-" + string(workerID) + "-" + src.Key()
}

// attachmentTime keeps the subscription's own creation time so the
// converted attachment sorts where the subscription did. A zero
// timestamp (a hand-poked row) falls back to a fixed non-zero instant
// rather than time.Now, so the conversion stays deterministic.
func attachmentTime(createdAt time.Time) time.Time {
	if createdAt.IsZero() {
		return time.Unix(0, 0).UTC()
	}
	return createdAt.UTC()
}
