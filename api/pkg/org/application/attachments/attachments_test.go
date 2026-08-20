package attachments_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/attachments"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/stretchr/testify/require"
)

func setup(t *testing.T) (context.Context, *store.Store, *attachments.Service) {
	t.Helper()
	ctx := context.Background()
	st := memory.New()
	node, err := orgchart.NewNode("w-one", "work", nil, time.Now(), "org-1")
	require.NoError(t, err)
	require.NoError(t, st.Nodes.Create(ctx, node))
	tr, err := trigger.New("tr-1", "org-1", "incoming", transport.KindLocal, nil, "", time.Now())
	require.NoError(t, err)
	require.NoError(t, st.Triggers.Create(ctx, tr))
	n := 0
	return ctx, st, attachments.New(attachments.Deps{Store: st, NewID: func() string { n++; return fmt.Sprint(n) }})
}
func TestCreateTriggerAndProcessorOutput(t *testing.T) {
	ctx, st, svc := setup(t)
	a, err := svc.Create(ctx, "org-1", "w-one", eventsource.Trigger("tr-1"), "")
	require.NoError(t, err)
	require.Equal(t, "wa-1", a.ID)
	p, err := processor.NewProcessor("p-1", "route", "", processor.KindTemplate, json.RawMessage(`{"template":"{{ .Message.body }}"}`), []processor.Output{{ID: "po-a", TopicID: "s-a"}}, "", time.Now(), "org-1")
	require.NoError(t, err)
	require.NoError(t, st.Processors.Create(ctx, p))
	_, err = svc.Create(ctx, "org-1", "w-one", eventsource.ProcessorOutput("p-1", "po-a"), "")
	require.NoError(t, err)
}
func TestCreateFailuresHaveContext(t *testing.T) {
	ctx, st, svc := setup(t)
	human, err := orgchart.NewNode("w-human", "person", nil, time.Now(), "org-1")
	require.NoError(t, err)
	require.NoError(t, st.Nodes.Create(ctx, human.WithKind(orgchart.NodeKindHuman)))
	p, err := processor.NewProcessor("p-1", "route", "", processor.KindTemplate, json.RawMessage(`{"template":"{{ .Message.body }}"}`), []processor.Output{{ID: "po-a", TopicID: "s-a"}}, "", time.Now(), "org-1")
	require.NoError(t, err)
	require.NoError(t, st.Processors.Create(ctx, p))
	cases := []struct {
		name, org, worker string
		src               eventsource.SourceRef
		contains          string
	}{{"worker missing", "org-1", "w-no", eventsource.Trigger("tr-1"), "get worker"}, {"cross tenant worker", "org-2", "w-one", eventsource.Trigger("tr-1"), "get worker"}, {"human", "org-1", "w-human", eventsource.Trigger("tr-1"), "is human"}, {"invalid source", "org-1", "w-one", eventsource.SourceRef{}, "unknown source kind"}, {"trigger missing", "org-1", "w-one", eventsource.Trigger("tr-no"), "trigger \"tr-no\""}, {"processor missing", "org-1", "w-one", eventsource.ProcessorOutput("p-no", "po"), "get processor"}, {"output missing", "org-1", "w-one", eventsource.ProcessorOutput("p-1", "po-no"), "output \"po-no\""}}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tt.org, orgchart.NodeID(tt.worker), tt.src, "")
			require.ErrorContains(t, err, tt.contains)
		})
	}
	_, err = svc.Create(ctx, "org-1", "w-one", eventsource.Trigger("tr-1"), "")
	require.NoError(t, err)
	_, err = svc.Create(ctx, "org-1", "w-one", eventsource.Trigger("tr-1"), "")
	require.Error(t, err)
	require.True(t, errors.Is(err, store.ErrConflict))
	require.ErrorContains(t, err, "persist")
}
func TestDeleteAndCleanup(t *testing.T) {
	ctx, st, svc := setup(t)
	a, err := svc.Create(ctx, "org-1", "w-one", eventsource.Trigger("tr-1"), "")
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, "org-1", a.ID))
	require.ErrorIs(t, svc.Delete(ctx, "org-1", a.ID), store.ErrNotFound)
	a, err = svc.Create(ctx, "org-1", "w-one", eventsource.Trigger("tr-1"), "")
	require.NoError(t, err)
	require.NoError(t, st.Nodes.Delete(ctx, "org-1", "w-one"))
	rows, err := st.WorkerAttachments.Find(ctx, store.WithOrg("org-1"))
	require.NoError(t, err)
	require.Empty(t, rows)
}
func TestMissingRepository(t *testing.T) {
	svc := attachments.New(attachments.Deps{Store: &store.Store{}})
	_, err := svc.Create(context.Background(), "org", "w", eventsource.Trigger("tr"), "")
	require.ErrorContains(t, err, "not configured")
	require.ErrorContains(t, svc.Delete(context.Background(), "org", "x"), "not configured")
}
