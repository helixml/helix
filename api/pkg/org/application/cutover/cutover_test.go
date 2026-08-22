package cutover_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/cutover"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/stretchr/testify/require"
)

const org = "org-cutover"

func at() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }

func topic(t *testing.T, id, name string, tr transport.Transport) streaming.Topic {
	t.Helper()
	row, err := streaming.NewTopic(id, name, "", "b-owner", at(), tr, org)
	require.NoError(t, err)
	return row
}

func sub(t *testing.T, worker, topicID string) streaming.Subscription {
	t.Helper()
	row, err := streaming.NewSubscription(worker, topicID, at(), org)
	require.NoError(t, err)
	return row
}

func worker(t *testing.T, st *store.Store, id string, human bool) {
	t.Helper()
	n, err := orgchart.NewNode(orgchart.NodeID(id), "# "+id, nil, at(), org)
	require.NoError(t, err)
	if human {
		n = n.WithKind(orgchart.NodeKindHuman)
	}
	require.NoError(t, st.Nodes.Create(context.Background(), n))
}

// seedBranch persists a Processor whose single branch writes to the given
// pre-cutover output Topic — the shape a pre-cutover Processor left
// behind once its branches were given durable ids.
func seedBranch(t *testing.T, st *store.Store, procID, outputID, outputStream string) processor.Processor {
	t.Helper()
	p, err := processor.NewProcessor(procID, procID, eventsource.SourceRef{}, processor.KindTruncate,
		[]byte(`{"max_bytes":100}`),
		[]processor.Output{{ID: outputID, StreamID: outputStream}},
		processor.SystemActor, at(), org)
	require.NoError(t, err)
	require.NoError(t, st.Processors.Create(context.Background(), p))
	return p
}

func triggerIDs(t *testing.T, st *store.Store) []string {
	t.Helper()
	rows, err := st.Triggers.Find(context.Background(), store.WithOrg(org), store.WithOrderAsc("id"))
	require.NoError(t, err)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

func attachmentSources(t *testing.T, st *store.Store, workerID string) []string {
	t.Helper()
	rows, err := st.WorkerAttachments.Find(context.Background(), store.WithOrg(org),
		store.WithWorkerID(orgchart.NodeID(workerID)), store.WithOrderAsc("id"))
	require.NoError(t, err)
	out := make([]string, 0, len(rows))
	for _, a := range rows {
		out = append(out, a.Source.Key())
	}
	return out
}

// TestConvert_TopicsBecomeSameIDTriggers is the history invariant: a
// converted Topic keeps its id, so every persisted event on that stream
// stays addressable and nothing is copied or rewritten.
func TestConvert_TopicsBecomeSameIDTriggers(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	ghCfg := []byte(`{"repo":"helixml/helix","events":["issues"]}`)
	memory.SeedRetired(st, []streaming.Topic{
		topic(t, "s-general", "general", transport.LocalTransport()),
		topic(t, "s-gh", "github", transport.Transport{Kind: transport.KindGitHub, Config: ghCfg}),
	}, nil, nil)

	res, err := cutover.Convert(ctx, cutover.Deps{Store: st})
	require.NoError(t, err)
	require.Equal(t, 2, res.Triggers)
	require.ElementsMatch(t, []string{"s-gh", "s-general"}, triggerIDs(t, st))

	// The inbound transport config survives; the Trigger is the source.
	rows, err := st.Triggers.Find(ctx, store.WithOrg(org), store.WithID("s-gh"), store.WithLimit(1))
	require.NoError(t, err)
	require.Equal(t, transport.KindGitHub, rows[0].Kind)
	cfg, err := rows[0].Transport().GitHubConfig()
	require.NoError(t, err)
	require.Equal(t, "helixml/helix", cfg.Repo)
}

// TestConvert_ProcessorOutputTopicIsNotATrigger: an output Topic was
// never an inbound source. It must not become a Trigger — the owning
// branch already carries it as a stream, and the branch's durable id is
// what Workers address.
func TestConvert_ProcessorOutputTopicIsNotATrigger(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	seedBranch(t, st, "p-truncate", "po-out", "s-truncated")
	memory.SeedRetired(st, []streaming.Topic{
		topic(t, "s-general", "general", transport.LocalTransport()),
		topic(t, "s-truncated", "p-truncate output", transport.LocalTransport()),
	}, nil, nil)

	res, err := cutover.Convert(ctx, cutover.Deps{Store: st})
	require.NoError(t, err)
	require.Equal(t, 1, res.Triggers)
	require.Equal(t, 1, res.Skipped)
	require.Equal(t, []string{"s-general"}, triggerIDs(t, st))
}

// TestConvert_SubscriptionsBecomeAttachments covers both shapes: a
// subscription to an ordinary Topic becomes an attachment to its same-id
// Trigger, and one to a Processor output becomes an attachment to that
// Processor's exact branch — which is what keeps a Worker receiving the
// results it was already receiving.
func TestConvert_SubscriptionsBecomeAttachments(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	worker(t, st, "b-sam", false)
	seedBranch(t, st, "p-truncate", "po-out", "s-truncated")
	memory.SeedRetired(st,
		[]streaming.Topic{
			topic(t, "s-general", "general", transport.LocalTransport()),
			topic(t, "s-truncated", "p-truncate output", transport.LocalTransport()),
		},
		[]streaming.Subscription{
			sub(t, "b-sam", "s-general"),
			sub(t, "b-sam", "s-truncated"),
		}, nil)

	res, err := cutover.Convert(ctx, cutover.Deps{Store: st})
	require.NoError(t, err)
	require.Equal(t, 2, res.Attachments)
	require.ElementsMatch(t,
		[]string{"trigger:s-general", "processor_output:p-truncate:po-out"},
		attachmentSources(t, st, "b-sam"))
}

// TestConvert_ProcessorInputsBecomeSourceRefs: a Processor's input Topic
// becomes a terminal reference — a Trigger for an ordinary Topic, the
// exact upstream branch for a Processor output.
func TestConvert_ProcessorInputsBecomeSourceRefs(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	upstream := seedBranch(t, st, "p-first", "po-first", "s-first-out")
	seedBranch(t, st, "p-second", "po-second", "s-second-out")
	memory.SeedRetired(st,
		[]streaming.Topic{
			topic(t, "s-in", "inbox", transport.LocalTransport()),
			topic(t, "s-first-out", "p-first output", transport.LocalTransport()),
		}, nil,
		map[store.OrgScopedID]streaming.StreamID{
			{OrgID: org, ID: "p-first"}:  "s-in",
			{OrgID: org, ID: "p-second"}: "s-first-out",
		})

	res, err := cutover.Convert(ctx, cutover.Deps{Store: st})
	require.NoError(t, err)
	require.Equal(t, 2, res.Inputs)

	first, err := st.Processors.Get(ctx, org, "p-first")
	require.NoError(t, err)
	require.Equal(t, eventsource.Trigger("s-in"), first.InputSource)

	second, err := st.Processors.Get(ctx, org, "p-second")
	require.NoError(t, err)
	require.Equal(t, upstream.Source(upstream.Outputs[0]), second.InputSource)
}

// TestConvert_IsRepeatSafe is the whole design constraint: running the
// conversion again on converged data does nothing at all.
func TestConvert_IsRepeatSafe(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	worker(t, st, "b-sam", false)
	seedBranch(t, st, "p-truncate", "po-out", "s-truncated")
	memory.SeedRetired(st,
		[]streaming.Topic{
			topic(t, "s-general", "general", transport.LocalTransport()),
			topic(t, "s-truncated", "p-truncate output", transport.LocalTransport()),
		},
		[]streaming.Subscription{sub(t, "b-sam", "s-general")},
		map[store.OrgScopedID]streaming.StreamID{{OrgID: org, ID: "p-truncate"}: "s-general"})

	first, err := cutover.Convert(ctx, cutover.Deps{Store: st})
	require.NoError(t, err)
	require.Equal(t, 1, first.Triggers)
	require.Equal(t, 1, first.Attachments)
	require.Equal(t, 1, first.Inputs)

	second, err := cutover.Convert(ctx, cutover.Deps{Store: st})
	require.NoError(t, err)
	require.Zero(t, second.Triggers)
	require.Zero(t, second.Attachments)
	require.Zero(t, second.Inputs)

	// And the end state is unchanged, not doubled.
	require.Equal(t, []string{"s-general"}, triggerIDs(t, st))
	require.Equal(t, []string{"trigger:s-general"}, attachmentSources(t, st, "b-sam"))
}

// TestConvert_SkipsDanglingAndHumanSubscriptions: a subscription whose
// Worker is gone, and one belonging to a human, are not failures — they
// simply have no attachment to become. A dangling row must not abort the
// whole upgrade.
func TestConvert_SkipsDanglingAndHumanSubscriptions(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	worker(t, st, "b-sam", false)
	worker(t, st, "h-alice", true)
	memory.SeedRetired(st,
		[]streaming.Topic{topic(t, "s-general", "general", transport.LocalTransport())},
		[]streaming.Subscription{
			sub(t, "b-sam", "s-general"),
			sub(t, "h-alice", "s-general"),
			sub(t, "b-departed", "s-general"),
			sub(t, "b-sam", "s-deleted-topic"),
		}, nil)

	res, err := cutover.Convert(ctx, cutover.Deps{Store: st})
	require.NoError(t, err)
	require.Equal(t, 1, res.Attachments)
	require.Equal(t, []string{"trigger:s-general"}, attachmentSources(t, st, "b-sam"))
	require.Empty(t, attachmentSources(t, st, "h-alice"))
}

// TestConvert_NothingToDoOnCleanInstall: a deployment that never ran a
// pre-cutover release reads three empty tables and returns.
func TestConvert_NothingToDoOnCleanInstall(t *testing.T) {
	res, err := cutover.Convert(context.Background(), cutover.Deps{Store: memory.New()})
	require.NoError(t, err)
	require.Zero(t, res.Triggers)
	require.Zero(t, res.Attachments)
	require.Zero(t, res.Inputs)
}
