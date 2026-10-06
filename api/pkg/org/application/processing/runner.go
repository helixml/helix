// Package processing is the execution arm for Processors: it turns a
// freshly-published event on a source into the outputs the Processors
// reading that source produce, and publishes each result back through
// the same publish→route backbone. It is wired into the Dispatcher as a
// late-bound fan-out arm (RegisterProcessorRunner) — the publishing
// service the Runner depends on is built *after* the Dispatcher at the
// composition root, so the dependency is injected, not constructed.
//
// The Runner does exactly one structural thing: list the processors
// reading a source, Process the message (a pure domain call), and
// publish each Result from the branch that produced it. No agent
// decisions, no implicit chaining — chaining falls out for free because
// a published branch event re-enters routing, which calls the Runner
// again on that branch's source. A hop guard bounds that recursion;
// create-time cycle checks (in application/processors) prevent the
// graph from looping in the first place.
package processing

import (
	"context"
	"log/slog"

	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Publisher is the narrow port the Runner needs to re-publish a
// processed Message from its output branch. publishing.Publishing
// satisfies it; declared here (not imported) so processing does not
// depend on publishing and the import edge stays one-way.
type Publisher interface {
	Publish(ctx context.Context, orgID string, src eventsource.SourceRef, streamID streaming.StreamID, from string, msg streaming.Message) (streaming.Event, error)
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

// PostRouter is a late-bound observer fired after an Automated processor
// routes a Message — the seam thread-follow plugs into. It receives the
// processor, the input Message, and the Results its pure Process produced
// (the name-matched routes), so it can record/extend routing with state the
// pure Process must not touch. Defined here (not imported) so processing
// stays Slack-agnostic; slackrouting.ThreadFollower satisfies it and is
// registered at the composition root, exactly like the ProcessorRunner /
// Outbound arms. nil → no post-routing, the default.
type PostRouter interface {
	AfterRoute(ctx context.Context, p processor.Processor, msg streaming.Message, results []processor.Result)
}

// Runner executes the processors that read a topic and publishes their
// results. Construct with New.
type Runner struct {
	procs      store.Processors
	publisher  Publisher
	logger     *slog.Logger
	postRouter PostRouter
}

// New constructs a Runner. logger must be non-nil.
func New(procs store.Processors, publisher Publisher, logger *slog.Logger) *Runner {
	return &Runner{procs: procs, publisher: publisher, logger: logger}
}

// RegisterPostRouter wires the post-routing observer (thread-follow).
// Late-bound for the same reason as the dispatcher's arms: the follower
// depends on the publishing service built after the Runner.
func (r *Runner) RegisterPostRouter(pr PostRouter) { r.postRouter = pr }

// Run is the Dispatcher's late-bound fan-out hook. For each processor
// reading e.Source it applies the processor to the message and publishes
// every Result from the branch that produced it. Errors are logged and
// the offending processor/result skipped (OQ3: log + drop), so one bad
// template never stalls the others or the originating publish.
//
// Branch output is published with an empty `from` -> Source=""
// (system-emitted). A template that needs the originator embeds
// {{ .Message.from }} in its body.
func (r *Runner) Run(ctx context.Context, e eventsource.Event) {
	if hop := hopCount(ctx); hop >= maxHops {
		r.logger.Error("processing: hop limit exceeded — aborting chain",
			"source", e.Source.Key(), "event", e.ID, "hops", hop)
		return
	}
	procs, err := r.procs.ListByInputSource(ctx, e.OrganizationID, e.Source)
	if err != nil {
		r.logger.Error("processing: list processors by input source", "source", e.Source.Key(), "err", err)
		return
	}
	for _, p := range procs {
		results, err := p.Process(ctx, e.Message)
		if err != nil {
			r.logger.Warn("processing: process failed — dropping",
				"processor", p.ID, "source", e.Source.Key(), "event", e.ID, "err", err)
			continue
		}
		for _, res := range results {
			if _, err := r.publisher.Publish(withHop(ctx), e.OrganizationID, p.Source(res.Output), res.Output.StreamID, "", res.Message); err != nil {
				r.logger.Warn("processing: publish result",
					"processor", p.ID, "output", res.Output.ID, "err", err)
			}
		}
		// Post-routing arm (thread-follow): only Automated processors carry
		// stateful routing, so the cheap field check gates the hop into the
		// Slack-aware follower. The follower extends delivery (to thread
		// members) using state Process is forbidden to read.
		if r.postRouter != nil && p.Automated() {
			r.postRouter.AfterRoute(ctx, p, e.Message, results)
		}
	}
}
