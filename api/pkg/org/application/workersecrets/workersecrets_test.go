package workersecrets_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/workersecrets"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

type resolver struct {
	values      map[string]string
	unavailable map[string]bool
	calls       int
}

func (r *resolver) Validate(_ context.Context, b workersecret.Binding) error {
	if r.unavailable[b.SecretID] {
		return errors.New("source unavailable")
	}
	return nil
}
func (r *resolver) Resolve(ctx context.Context, b workersecret.Binding) (workersecret.Resolved, error) {
	r.calls++
	if err := r.Validate(ctx, b); err != nil {
		return workersecret.Resolved{}, err
	}
	return workersecret.Resolved{Value: r.values[b.SecretID], Descriptor: workersecret.Descriptor{Available: true}}, nil
}

func TestWorkerSecretServiceResolvesLiveAndListsMetadataOnly(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	node, err := orgchart.NewNode("w-1", "worker", nil, now, "org-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}
	r := &resolver{values: map[string]string{"sec-1": "first"}, unavailable: map[string]bool{}}
	svc, err := workersecrets.New(st.WorkerSecretBindings, st.Nodes, r, func() time.Time { return now }, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Put(ctx, workersecret.Binding{OrganizationID: "org-1", WorkerID: "w-1", Name: "API_TOKEN", SourceKind: workersecret.SourceHelixSecret, SecretID: "sec-1", Usage: "export API_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	descriptors, err := svc.Descriptors(ctx, "org-1", "w-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(descriptors) != 1 || descriptors[0].Name != "API_TOKEN" || !descriptors[0].Available {
		t.Fatalf("descriptors=%+v", descriptors)
	}
	res, err := svc.Get(ctx, "org-1", "w-1", "API_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != "first" {
		t.Fatalf("value=%q", res.Value)
	}
	r.values["sec-1"] = "rotated"
	res, err = svc.Get(ctx, "org-1", "w-1", "API_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != "rotated" || r.calls != 2 {
		t.Fatalf("result=%+v calls=%d", res, r.calls)
	}
}

func TestWorkerSecretServiceRejectsReservedAndCrossWorkerNames(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Now()
	node, _ := orgchart.NewNode("w-1", "worker", nil, now, "org-1")
	_ = st.Nodes.Create(ctx, node)
	r := &resolver{values: map[string]string{}, unavailable: map[string]bool{}}
	svc, _ := workersecrets.New(st.WorkerSecretBindings, st.Nodes, r, time.Now, nil)
	for _, name := range []string{"USER_API_TOKEN", "HELIX_API_URL"} {
		if _, err := svc.Put(ctx, workersecret.Binding{OrganizationID: "org-1", WorkerID: "w-1", Name: name, SourceKind: workersecret.SourceHelixSecret, SecretID: "s"}); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := svc.Get(ctx, "org-1", "w-2", "API_TOKEN"); err == nil {
		t.Fatal("cross-worker lookup accepted")
	}
}
