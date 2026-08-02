package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/types"
)

type orgAuditStore struct {
	entry *types.OrgAuditLog
}

func (s *orgAuditStore) CreateOrgAuditLog(_ context.Context, entry *types.OrgAuditLog) error {
	s.entry = entry
	return nil
}

func TestOrgAuditLogServiceRecord(t *testing.T) {
	store := &orgAuditStore{}
	service := NewOrgAuditLogService(store)
	createdAt := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return createdAt }
	service.newID = func() string { return "audit-1" }

	err := service.Record(context.Background(), orgaudit.Entry{
		OrganizationID: "org-1",
		ProjectID:      "project-1",
		UserID:         "user-1",
		ActorID:        "bot-1",
		ActorType:      orgaudit.ActorBot,
		AssetID:        "asset-1",
		EventType:      orgaudit.EventSSHCommand,
		Action:         "exec",
		Status:         orgaudit.StatusSucceeded,
		Metadata: orgaudit.Metadata{
			Command:   "uname -a",
			CommandID: "command-1",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "audit-1", store.entry.ID)
	require.Equal(t, "org-1", store.entry.OrganizationID)
	require.Equal(t, "project-1", store.entry.ProjectID)
	require.Equal(t, "user-1", store.entry.UserID)
	require.Equal(t, "bot-1", store.entry.ActorID)
	require.Equal(t, types.OrgAuditActorBot, store.entry.ActorType)
	require.Equal(t, "asset-1", store.entry.AssetID)
	require.Equal(t, types.OrgAuditEventSSHCommand, store.entry.EventType)
	require.Equal(t, types.OrgAuditStatusSucceeded, store.entry.Status)
	require.Equal(t, "uname -a", store.entry.Metadata.Command)
	require.Equal(t, createdAt, store.entry.CreatedAt)
}
