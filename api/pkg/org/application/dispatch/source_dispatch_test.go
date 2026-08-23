package dispatch_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/dispatch"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/attachment"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/stretchr/testify/require"
)

type queued struct {
	org     string
	worker  orgchart.NodeID
	trigger activation.Trigger
}
type recordingQueue struct{ rows []queued }

func (q *recordingQueue) Enqueue(org string, worker orgchart.NodeID, tr activation.Trigger) {
	q.rows = append(q.rows, queued{org, worker, tr})
}
func addNode(t *testing.T, ctx context.Context, st interface {
	Create(context.Context, orgchart.Node) error
}, org string, id orgchart.NodeID, human bool) {
	n, err := orgchart.NewNode(id, "work", nil, time.Now(), org)
	require.NoError(t, err)
	if human {
		n = n.WithKind(orgchart.NodeKindHuman)
	}
	require.NoError(t, st.Create(ctx, n))
}
func addAttachment(t *testing.T, ctx context.Context, repo interface {
	Create(context.Context, attachment.Attachment) error
}, id, org string, worker orgchart.NodeID, src eventsource.SourceRef) {
	a, err := attachment.New(id, org, worker, src, "", time.Now())
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, a))
}

func TestDispatchSourceExactFanoutOrderingAndSuppression(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	addNode(t, ctx, st.Nodes, "org-1", "w-a", false)
	addNode(t, ctx, st.Nodes, "org-1", "w-b", false)
	addNode(t, ctx, st.Nodes, "org-1", "w-human", true)
	addNode(t, ctx, st.Nodes, "org-2", "w-a", false)
	src := eventsource.ProcessorOutput("p-1", "po-left")
	addAttachment(t, ctx, st.WorkerAttachments, "a-1", "org-1", "w-a", src)
	addAttachment(t, ctx, st.WorkerAttachments, "a-2", "org-1", "w-b", src)
	addAttachment(t, ctx, st.WorkerAttachments, "a-3", "org-1", "w-human", src)
	addAttachment(t, ctx, st.WorkerAttachments, "a-4", "org-1", "w-missing", src)
	addAttachment(t, ctx, st.WorkerAttachments, "a-5", "org-1", "w-b", eventsource.ProcessorOutput("p-1", "po-right"))
	addAttachment(t, ctx, st.WorkerAttachments, "a-6", "org-2", "w-a", src)
	d := dispatch.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := &recordingQueue{}
	d.RegisterActivationQueue(q)
	e, err := eventsource.NewEvent("e-1", "org-1", src, streaming.Message{Body: "one"}, "w-a", time.Now())
	require.NoError(t, err)
	require.NoError(t, d.DispatchSource(ctx, e))
	require.Len(t, q.rows, 1)
	require.Equal(t, orgchart.NodeID("w-b"), q.rows[0].worker)
	require.Equal(t, src, q.rows[0].trigger.EventSource)
	require.Equal(t, "one", q.rows[0].trigger.Message.Body)
	e2, err := eventsource.NewEvent("e-2", "org-1", src, streaming.Message{Body: "two"}, "", time.Now())
	require.NoError(t, err)
	require.NoError(t, d.DispatchSource(ctx, e2))
	require.Equal(t, []orgchart.NodeID{"w-b", "w-a", "w-b"}, []orgchart.NodeID{q.rows[0].worker, q.rows[1].worker, q.rows[2].worker})
	require.Equal(t, "two", q.rows[1].trigger.Message.Body)
}
func TestDispatchSourceMissingRepository(t *testing.T) {
	d := dispatch.New(&store.Store{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	err := d.DispatchSource(context.Background(), eventsource.Event{})
	require.ErrorContains(t, err, "not configured")
}
