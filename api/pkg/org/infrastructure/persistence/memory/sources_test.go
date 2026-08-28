package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/stretchr/testify/require"
)

func TestTriggersCRUDQueriesAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	a, err := trigger.New("tr-same", "org-a", "A", "Original description", transport.KindLocal, nil, "", time.Now())
	require.NoError(t, err)
	b, err := trigger.New("tr-same", "org-b", "B", "", transport.KindLocal, nil, "", time.Now())
	require.NoError(t, err)
	require.NoError(t, st.Triggers.Create(ctx, a))
	require.NoError(t, st.Triggers.Create(ctx, b))
	rows, err := st.Triggers.Find(ctx, store.WithOrg("org-a"), store.WithID("tr-same"))
	require.NoError(t, err)
	require.Equal(t, []trigger.Trigger{a}, rows)
	a.Name = "Updated"
	a.Description = "Updated description"
	require.NoError(t, st.Triggers.Update(ctx, a))
	rows, err = st.Triggers.Find(ctx, store.WithOrg("org-a"), store.WithTransportKind(transport.KindLocal), store.WithLimit(1))
	require.NoError(t, err)
	require.Equal(t, "Updated", rows[0].Name)
	require.Equal(t, "Updated description", rows[0].Description)
	duplicate, err := trigger.New("tr-other", "org-a", "Updated", "", transport.KindLocal, nil, "", time.Now())
	require.NoError(t, err)
	require.ErrorIs(t, st.Triggers.Create(ctx, duplicate), store.ErrConflict)
	require.NoError(t, st.Triggers.Delete(ctx, "org-a", "tr-same"))
	rows, err = st.Triggers.Find(ctx, store.WithOrg("org-b"), store.WithID("tr-same"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, errors.Is(st.Triggers.Delete(ctx, "org-a", "tr-same"), store.ErrNotFound))
}
