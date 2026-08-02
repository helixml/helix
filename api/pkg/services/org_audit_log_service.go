package services

import (
	"context"
	"time"

	"github.com/google/uuid"

	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/types"
)

type OrgAuditLogStore interface {
	CreateOrgAuditLog(ctx context.Context, entry *types.OrgAuditLog) error
}

type OrgAuditLogService struct {
	store OrgAuditLogStore
	now   func() time.Time
	newID func() string
}

func NewOrgAuditLogService(store OrgAuditLogStore) *OrgAuditLogService {
	return &OrgAuditLogService{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
		newID: uuid.NewString,
	}
}

func (s *OrgAuditLogService) Record(ctx context.Context, entry orgaudit.Entry) error {
	return s.store.CreateOrgAuditLog(ctx, &types.OrgAuditLog{
		ID:             s.newID(),
		OrganizationID: entry.OrganizationID,
		ProjectID:      entry.ProjectID,
		UserID:         entry.UserID,
		ActorID:        entry.ActorID,
		ActorType:      types.OrgAuditActorType(entry.ActorType),
		AssetID:        entry.AssetID,
		EventType:      types.OrgAuditEventType(entry.EventType),
		Action:         entry.Action,
		Status:         types.OrgAuditStatus(entry.Status),
		Metadata: types.OrgAuditMetadata{
			Arguments:     entry.Metadata.Arguments,
			AssetRef:      entry.Metadata.AssetRef,
			Command:       entry.Metadata.Command,
			CommandID:     entry.Metadata.CommandID,
			Error:         entry.Metadata.Error,
			RemoteAddress: entry.Metadata.RemoteAddress,
			LocalAddress:  entry.Metadata.LocalAddress,
			SSHUser:       entry.Metadata.SSHUser,
			ClientVersion: entry.Metadata.ClientVersion,
			DurationMS:    entry.Metadata.DurationMS,
		},
		CreatedAt: s.now(),
	})
}
