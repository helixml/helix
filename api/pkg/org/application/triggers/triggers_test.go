package triggers_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/attachments"
	triggerapp "github.com/helixml/helix/api/pkg/org/application/triggers"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/stretchr/testify/require"
)

func TestCRUDRevisionAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	ids := []string{"one", "two"}
	newID := func() string { id := ids[0]; ids = ids[1:]; return id }
	svc := triggerapp.New(triggerapp.Deps{Triggers: st.Triggers, Attachments: st.WorkerAttachments, Events: st.Events, NewID: newID, Now: func() time.Time { return time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC) }})

	created, err := svc.Create(ctx, "org-a", triggerapp.CreateParams{Name: "Incoming", Kind: transport.KindLocal})
	require.NoError(t, err)
	_, err = svc.Get(ctx, "org-b", created.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = svc.Update(ctx, "org-a", created.ID, "stale", triggerapp.UpdateParams{Name: "Lost", Kind: transport.KindLocal})
	require.ErrorIs(t, err, store.ErrConflict)
	updated, err := svc.Update(ctx, "org-a", created.ID, triggerapp.Revision(created), triggerapp.UpdateParams{Name: "Renamed", Kind: transport.KindLocal})
	require.NoError(t, err)
	require.Equal(t, "Renamed", updated.Name)
}

func TestDeleteRejectsAttachedTrigger(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	node, err := orgchart.NewNode("w-one", "# Worker", nil, time.Now(), "org-a")
	require.NoError(t, err)
	require.NoError(t, st.Nodes.Create(ctx, node))
	svc := triggerapp.New(triggerapp.Deps{Triggers: st.Triggers, Attachments: st.WorkerAttachments, Events: st.Events, NewID: func() string { return "one" }})
	created, err := svc.Create(ctx, "org-a", triggerapp.CreateParams{Name: "Incoming", Kind: transport.KindLocal})
	require.NoError(t, err)
	attachmentSvc := attachments.New(attachments.Deps{Store: st, NewID: func() string { return "one" }})
	_, err = attachmentSvc.Create(ctx, "org-a", "w-one", eventsource.Trigger(created.ID), "")
	require.NoError(t, err)
	require.True(t, errors.Is(svc.Delete(ctx, "org-a", created.ID), triggerapp.ErrSourceInUse))
}
