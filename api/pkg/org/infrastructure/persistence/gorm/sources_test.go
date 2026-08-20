package gorm

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/attachment"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	gormio "gorm.io/gorm"
)

func TestTriggerAndAttachmentGORMRoundTrip(t *testing.T) {
	db, err := gormio.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gormio.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&triggerRow{}, &attachmentRow{}))
	require.NoError(t, installAttachmentConstraints(db))
	ctx := context.Background()
	triggers := newTriggersRepo(db)
	attachments := newAttachmentsRepo(db)
	tr, err := trigger.New("tr-same", "org-a", "Incoming", transport.KindLocal, nil, "", time.Now())
	require.NoError(t, err)
	require.NoError(t, triggers.Create(ctx, tr))
	other, err := trigger.New("tr-same", "org-b", "Incoming", transport.KindLocal, nil, "", time.Now())
	require.NoError(t, err)
	require.NoError(t, triggers.Create(ctx, other))
	rows, err := triggers.Find(ctx, store.WithOrg("org-a"), store.WithID("tr-same"))
	require.NoError(t, err)
	require.Equal(t, tr, rows[0])
	a, err := attachment.New("wa-1", "org-a", orgchart.NodeID("w-1"), eventsource.Trigger("tr-same"), "", time.Now())
	require.NoError(t, err)
	require.NoError(t, attachments.Create(ctx, a))
	found, err := attachments.Find(ctx, store.WithOrg("org-a"), store.WithTriggerID("tr-same"))
	require.NoError(t, err)
	require.Equal(t, a, found[0])
	duplicate, err := attachment.New("wa-2", "org-a", orgchart.NodeID("w-1"), eventsource.Trigger("tr-same"), "", time.Now())
	require.NoError(t, err)
	require.ErrorIs(t, attachments.Create(ctx, duplicate), store.ErrConflict)
}

func TestProcessorOutputIDUpgradeIsIdempotent(t *testing.T) {
	db, err := gormio.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gormio.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&processorRow{}))
	row := processorRow{ID: "p-1", OrgID: "org-1", Name: "P", Kind: "template", Config: `{"template":"x"}`, Outputs: `[{"TopicID":"s-out","Owned":true}]`, CreatedAt: time.Now()}
	require.NoError(t, db.Create(&row).Error)
	require.NoError(t, migrateProcessorOutputIDs(db))
	require.NoError(t, migrateProcessorOutputIDs(db))
	var got processorRow
	require.NoError(t, db.First(&got, "id = ? AND org_id = ?", "p-1", "org-1").Error)
	outputs, err := unmarshalProcessorOutputs([]byte(got.Outputs))
	require.NoError(t, err)
	require.Equal(t, "po-topic-s-out", outputs[0].ID)
}
