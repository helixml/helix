package lifecycle_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

type recordingAudit struct {
	mu      sync.Mutex
	entries []audit.Entry
	err     error
}

func (r *recordingAudit) Record(_ context.Context, e audit.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.entries = append(r.entries, e)
	return nil
}

func (r *recordingAudit) statuses() []audit.Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Status, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.Status)
	}
	return out
}

func requireStatuses(t *testing.T, rec *recordingAudit, want ...audit.Status) {
	t.Helper()
	got := rec.statuses()
	if len(got) != len(want) {
		t.Fatalf("audit statuses = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("audit statuses = %v, want %v", got, want)
		}
	}
}

func TestDelete_AuditsAttemptAndSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-delete-audit"
	seedBot(t, st, orgID, "w-audited")

	rec := &recordingAudit{}
	svc := &lifecycle.Service{Store: st, Audit: rec}
	actor := lifecycle.Actor{Type: audit.ActorUser, ID: "u-luke"}
	if err := svc.Delete(ctx, orgID, "w-audited", actor); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	requireStatuses(t, rec, audit.StatusAttempted, audit.StatusSucceeded)
	for _, e := range rec.entries {
		if e.Action != "delete_bot" || e.EventType != audit.EventBotLifecycle ||
			e.AssetID != "w-audited" || e.OrganizationID != orgID ||
			e.ActorType != audit.ActorUser || e.ActorID != "u-luke" || e.UserID != "u-luke" {
			t.Fatalf("unexpected audit entry: %+v", e)
		}
	}
}

func TestDelete_AuditsFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-delete-audit-fail"
	seedBot(t, st, orgID, "w-fails")

	rec := &recordingAudit{}
	svc := &lifecycle.Service{Store: st, Audit: rec, Instances: blockingDeleter{err: errors.New("host unreachable")}}
	if err := svc.Delete(ctx, orgID, "w-fails", lifecycle.Actor{Type: audit.ActorBot, ID: "b-manager"}); err == nil {
		t.Fatal("Delete should fail")
	}

	requireStatuses(t, rec, audit.StatusAttempted, audit.StatusFailed)
	failed := rec.entries[1]
	if failed.Metadata.Error == "" || failed.UserID != "" || failed.ActorID != "b-manager" {
		t.Fatalf("failed entry = %+v, want bot actor and error", failed)
	}
}

// An unauditable delete must not run: the audit trail is the only record of
// when and by whom a bot was removed.
func TestDelete_RefusesWhenAuditUnavailable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-delete-audit-down"
	seedBot(t, st, orgID, "w-kept")

	svc := &lifecycle.Service{Store: st, Audit: &recordingAudit{err: errors.New("audit store down")}}
	if err := svc.Delete(ctx, orgID, "w-kept", lifecycle.Actor{Type: audit.ActorUser, ID: "u-1"}); err == nil {
		t.Fatal("Delete should refuse without an audit record")
	}
	if err := svc.StartDelete(ctx, orgID, "w-kept", lifecycle.Actor{Type: audit.ActorUser, ID: "u-1"}); err == nil {
		t.Fatal("StartDelete should refuse without an audit record")
	}
	svc.WaitDeletes()
	if svc.Deleting(orgID, "w-kept") {
		t.Fatal("refused StartDelete left the deleting marker set")
	}
	if _, err := st.Nodes.Get(ctx, orgID, "w-kept"); err != nil {
		t.Fatalf("bot should survive a refused delete: %v", err)
	}
}

func TestStartDelete_AuditsBackgroundOutcome(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-start-delete-audit"
	seedBot(t, st, orgID, "w-async-audit")

	rec := &recordingAudit{}
	svc := &lifecycle.Service{Store: st, Audit: rec}
	if err := svc.StartDelete(ctx, orgID, "w-async-audit", lifecycle.Actor{Type: audit.ActorUser, ID: "u-1"}); err != nil {
		t.Fatalf("StartDelete: %v", err)
	}
	svc.WaitDeletes()

	requireStatuses(t, rec, audit.StatusAttempted, audit.StatusSucceeded)
	if _, err := st.Nodes.Get(ctx, orgID, "w-async-audit"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("node after delete: err=%v, want ErrNotFound", err)
	}
}
