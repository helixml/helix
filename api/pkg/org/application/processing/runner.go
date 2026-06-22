// Package processing is the execution arm for Processors: it turns a
// freshly-published Event on a Topic into the Processor outputs that
// Topic feeds, and publishes each result back through the same
// publish→dispatch backbone. It is wired into the Dispatcher as a
// late-bound fan-out arm (RegisterProcessorRunner), mirroring how
// outbound emitters are registered — the publishing service that the
// Runner depends on is built *after* the Dispatcher at the composition
// root, so the dependency is injected, not constructed.
//
// The Runner does exactly one structural thing: list the processors
// reading a topic, Process the message (a pure domain call), and
// publish each Result. No agent decisions, no implicit
// subscribing/chaining — chaining falls out for free because a
// published output Event re-enters Dispatch, which calls the Runner
// again on the output topic. A hop guard bounds that recursion;
// create-time cycle checks (in application/processors) prevent the
// graph from looping in the first place.
package processing

import (
	"context"
	"log/slog"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Publisher is the narrow port the Runner needs to re-publish a
// processed Message onto its output Topic. publishing.Publishing
// satisfies it; declared here (not imported) so processing does not
// depend on publishing and the import edge stays one-way.
type Publisher interface {
	Publish(ctx context.Context, orgID string, topicID streaming.TopicID, from string, msg streaming.Message) (streaming.Event, error)
}

// maxHops bounds processor-chain recursion as defence-in-depth behind
// the create-time cycle check. A chain deeper than this aborts rather
// than looping forever. Real chains are a handful of hops; 10 is
// generous.
const maxHops = 10

type hopCtxKey struct{}

// hopCount reads the current processor-chain depth from ctx (0 when
// unset — the original publish).
func hopCount(ctx context.Context) int {
	if v, ok := ctx.Value(hopCtxKey{}).(int); ok {
		return v
	}
	return 0
}

// withHop returns ctx carrying an incremented hop count, threaded into
// the Publish that produces the next event so the next Runner pass sees
// the deeper depth.
func withHop(ctx context.Context) context.Context {
	return context.WithValue(ctx, hopCtxKey{}, hopCount(ctx)+1)
}

// Runner executes the processors that read a topic and publishes their
// results. Construct with New.
type Runner struct {
	procs     store.Processors
	publisher Publisher
	logger    *slog.Logger
}

// New constructs a Runner. logger must be non-nil.
func New(procs store.Processors, publisher Publisher, logger *slog.Logger) *Runner {
	return &Runner{procs: procs, publisher: publisher, logger: logger}
}

// Run is the Dispatcher's late-bound fan-out hook. For each processor
// reading e.TopicID it applies the processor to msg and publishes every
// Result onto the processor's output topic. Errors are logged and the
// offending processor/result skipped (OQ3: log + drop), so one bad
// template never stalls the others or the originating publish.
//
// Output is published with an empty `from` → Source="" (system-emitted):
// the processed event is never treated as worker-sourced and is never
// re-emitted outbound. A template that needs the originator embeds
// {{ .Message.from }} in its body (the flagship example does exactly
// that).
func (r *Runner) Run(ctx context.Context, e streaming.Event, msg streaming.Message) {
	if hop := hopCount(ctx); hop >= maxHops {
		r.logger.Error("processing: hop limit exceeded — aborting chain",
			"topic", e.TopicID, "event", e.ID, "hops", hop)
		return
	}
	procs, err := r.procs.ListByInputTopic(ctx, e.OrganizationID, e.TopicID)
	if err != nil {
		r.logger.Error("processing: list processors by input topic", "topic", e.TopicID, "err", err)
		return
	}
	for _, p := range procs {
		results, err := p.Process(msg)
		if err != nil {
			r.logger.Warn("processing: process failed — dropping",
				"processor", p.ID, "topic", e.TopicID, "event", e.ID, "err", err)
			continue
		}
		for _, res := range results {
			if _, err := r.publisher.Publish(withHop(ctx), e.OrganizationID, res.TopicID, "", res.Message); err != nil {
				r.logger.Warn("processing: publish result",
					"processor", p.ID, "output_topic", res.TopicID, "err", err)
			}
		}
	}
}
