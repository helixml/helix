package gorm_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
)

func TestWorkerSecretUpdateReplacesSourceAndClearsMetadata(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	worker, err := orgchart.NewNode("w-secret-update", "worker", nil, now, "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Nodes.Create(ctx, worker); err != nil {
		t.Fatal(err)
	}
	binding := workersecret.Binding{
		OrganizationID: "org-test", WorkerID: worker.ID, Name: "TOKEN",
		Description: "old description", Usage: "old usage",
		SourceKind: workersecret.SourceConnectedAccount,
		AccountID:  "account-1", ExportKey: "oauth/access_token",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.WorkerSecretBindings.Create(ctx, binding); err != nil {
		t.Fatal(err)
	}
	binding.Description = ""
	binding.Usage = ""
	binding.SourceKind = workersecret.SourceHelixSecret
	binding.SecretID = "secret-1"
	binding.AccountID = ""
	binding.ExportKey = ""
	binding.UpdatedAt = now.Add(time.Minute)
	if err := store.WorkerSecretBindings.Update(ctx, binding); err != nil {
		t.Fatal(err)
	}
	got, err := store.WorkerSecretBindings.Get(ctx, "org-test", worker.ID, "TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceKind != workersecret.SourceHelixSecret || got.SecretID != "secret-1" || got.AccountID != "" || got.ExportKey != "" {
		t.Fatalf("source was not replaced: %+v", got)
	}
	if got.Description != "" || got.Usage != "" {
		t.Fatalf("metadata was not cleared: %+v", got)
	}
}
