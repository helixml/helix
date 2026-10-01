package workersecrets_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/workersecrets"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

type resolver struct {
	values      map[string]string
	unavailable map[string]bool
	calls       int
	validations int
	expiresAt   *time.Time
	resourceID  string
}

func (r *resolver) Validate(_ context.Context, b workersecret.Binding) error {
	r.validations++
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
	return workersecret.Resolved{Value: r.values[b.SecretID], Descriptor: workersecret.Descriptor{Available: true, Usage: "provider usage", ExpiresAt: r.expiresAt, ResourceID: r.resourceID}}, nil
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
	for _, name := range []string{"USER_API_TOKEN", "user_api_token", "HELIX_API_URL", "helix_api_url", "BAD NAME", "BAD=NAME", "BAD/NAME"} {
		if _, err := svc.Put(ctx, workersecret.Binding{OrganizationID: "org-1", WorkerID: "w-1", Name: name, SourceKind: workersecret.SourceHelixSecret, SecretID: "s"}); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := svc.Get(ctx, "org-1", "w-2", "API_TOKEN"); err == nil {
		t.Fatal("cross-worker lookup accepted")
	}
}

func TestWorkerSecretServicePreservesBindingMetadataAndAddsSourceMetadata(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	node, _ := orgchart.NewNode("w-1", "worker", nil, now, "org-1")
	if err := st.Nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}
	expiresAt := now.Add(time.Hour)
	r := &resolver{values: map[string]string{"": "token"}, unavailable: map[string]bool{}, expiresAt: &expiresAt, resourceID: "T123"}
	catalog := func(context.Context, string, orgchart.NodeID) ([]workersecret.AvailableSource, error) {
		return []workersecret.AvailableSource{{
			SourceKind: workersecret.SourceConnectedAccount, AccountID: "account-1",
			ExportKey: "slack_workspace/bot_token", ResourceID: "T123",
		}}, nil
	}
	svc, _ := workersecrets.New(st.WorkerSecretBindings, st.Nodes, r, func() time.Time { return now }, nil, catalog)
	binding := workersecret.Binding{
		OrganizationID: "org-1", WorkerID: "w-1", Name: "SLACK_BOT_TOKEN",
		Description: "workspace token", Usage: "custom usage", ContentType: "text/plain",
		SuggestedFilename: "slack-token", SourceKind: workersecret.SourceConnectedAccount,
		AccountID: "account-1", ExportKey: "slack_workspace/bot_token",
	}
	if _, err := svc.Put(ctx, binding); err != nil {
		t.Fatal(err)
	}
	// Simulate provider-owned dynamic metadata that must not overwrite the binding.
	r.values[""] = "token"
	resolved, err := svc.Get(ctx, "org-1", "w-1", "SLACK_BOT_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Description != "workspace token" || resolved.Usage != "custom usage" || resolved.ContentType != "text/plain" || resolved.SuggestedFilename != "slack-token" || resolved.ResourceID != "T123" || resolved.ExpiresAt == nil || !resolved.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("resolved metadata = %+v", resolved.Descriptor)
	}
	descriptors, err := svc.Descriptors(ctx, "org-1", "w-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(descriptors) != 1 || !descriptors[0].Available || descriptors[0].ResourceID != "T123" {
		t.Fatalf("descriptors = %+v", descriptors)
	}
}

// A get_secret miss is the moment an agent decides whether to keep looking, so
// the not-found error has to stay machine-detectable (errors.Is) while telling
// the reader what the miss actually means. Asserting both stops a future
// refactor from quietly dropping either half.
func TestWorkerSecretServiceGetNotFoundKeepsSentinelAndExplainsMiss(t *testing.T) {
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
	r := &resolver{values: map[string]string{}, unavailable: map[string]bool{}}
	svc, err := workersecrets.New(st.WorkerSecretBindings, st.Nodes, r, func() time.Time { return now }, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Get(ctx, "org-1", "w-1", "ENV_ONLY_TOKEN")
	if err == nil {
		t.Fatal("expected an error for an unbound name")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("errors.Is(err, store.ErrNotFound) = false; err = %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "ENV_ONLY_TOKEN") {
		t.Errorf("message should name the credential; got %q", msg)
	}
	if !strings.Contains(msg, "environment") {
		t.Errorf("message should point at the container environment; got %q", msg)
	}
	// The value must never be printed, so the guidance must not tell the agent to.
	if strings.Contains(msg, "printenv") || strings.Contains(msg, "echo $") {
		t.Errorf("message must not suggest printing the value; got %q", msg)
	}
	if r.calls != 0 {
		t.Errorf("resolver should not be called when no binding exists; calls=%d", r.calls)
	}
}
