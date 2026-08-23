// Package orgtest holds the small fixture helpers the org tests share.
// It exists so the many tests that need "a Trigger exists" or "this
// Worker is attached to that source" express it once, the same way, and
// stay readable after the Topic cutover.
package orgtest

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
)

// Trigger creates a local Trigger with the given id, using the id as its
// name so tests never trip the org-scoped name uniqueness index.
func Trigger(t *testing.T, s *store.Store, orgID, id string) trigger.Trigger {
	t.Helper()
	return TriggerOfKind(t, s, orgID, id, transport.KindLocal, nil)
}

// TriggerOfKind creates a Trigger with an explicit transport kind and
// config — the shape inbound-transport tests need.
func TriggerOfKind(t *testing.T, s *store.Store, orgID, id string, kind transport.Kind, config []byte) trigger.Trigger {
	t.Helper()
	row, err := trigger.New(id, orgID, id, "", kind, config, "", time.Now().UTC())
	if err != nil {
		t.Fatalf("build trigger %q: %v", id, err)
	}
	if err := s.Triggers.Create(context.Background(), row); err != nil {
		t.Fatalf("create trigger %q: %v", id, err)
	}
	return row
}

// Attach attaches a Worker to a source, mirroring what the attachment
// service does but without needing the service wired.
func Attach(t *testing.T, s *store.Store, orgID string, workerID orgchart.NodeID, src eventsource.SourceRef) attachment.Attachment {
	t.Helper()
	a, err := attachment.New("wa-"+string(workerID)+"-"+src.Key(), orgID, workerID, src, "", time.Now().UTC())
	if err != nil {
		t.Fatalf("build attachment %s→%s: %v", workerID, src.Key(), err)
	}
	if err := s.WorkerAttachments.Create(context.Background(), a); err != nil {
		t.Fatalf("create attachment %s→%s: %v", workerID, src.Key(), err)
	}
	return a
}

// AttachTrigger is the common case: attach a Worker to a Trigger by id.
func AttachTrigger(t *testing.T, s *store.Store, orgID string, workerID orgchart.NodeID, triggerID string) attachment.Attachment {
	t.Helper()
	return Attach(t, s, orgID, workerID, eventsource.Trigger(triggerID))
}
