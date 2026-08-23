package processors_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/domain/attachment"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/org/internal/orgtest"
	"github.com/stretchr/testify/require"
)

const org = "org-1"

func tmplCfg(tmpl string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"template": tmpl})
	return b
}

func setup(t *testing.T) (*store.Store, *processors.Processors) {
	t.Helper()
	var n int
	id := func() string { n++; return time.Now().Format("150405.000") + "-" + string(rune('a'+n%26)) }
	s := memory.New()
	svc := processors.New(processors.Deps{Processors: s.Processors, Triggers: s.Triggers, Attachments: s.WorkerAttachments, NewID: id})
	return s, svc
}

// TestCreateAllocatesBranchIdentityAndStream: every branch gets a
// durable id (what attachments address) and its own event stream (what
// carries its history). Neither is caller-supplied by default.
func TestCreateAllocatesBranchIdentityAndStream(t *testing.T) {
	ctx := context.Background()
	s, svc := setup(t)
	orgtest.Trigger(t, s, org, "s-in")

	p, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Fmt", InputSource: eventsource.Trigger("s-in"), Kind: processor.KindTemplate,
		Config: tmplCfg("{{ .Message.body }}"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(p.Outputs) != 1 || p.Outputs[0].ID == "" || p.Outputs[0].StreamID == "" {
		t.Fatalf("expected one branch with an id and a stream, got %+v", p.Outputs)
	}
	if p.Source(p.Outputs[0]).Key() != "processor_output:"+p.ID+":"+p.Outputs[0].ID {
		t.Errorf("branch source = %q", p.Source(p.Outputs[0]).Key())
	}
	// Processor id minted with p- prefix.
	if len(p.ID) < 2 || p.ID[:2] != "p-" {
		t.Errorf("processor id = %q, want p- prefix", p.ID)
	}
}

// TestCreateAcceptsProcessorOutputInput: a Processor may read another
// Processor's exact branch, which is what makes a chain expressible.
func TestCreateAcceptsProcessorOutputInput(t *testing.T) {
	ctx := context.Background()
	s, svc := setup(t)
	orgtest.Trigger(t, s, org, "s-in")

	first, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "First", InputSource: eventsource.Trigger("s-in"), Kind: processor.KindTemplate,
		Config: tmplCfg("{{ .Message.body }}"),
	})
	require.NoError(t, err)

	second, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Second", InputSource: first.Source(first.Outputs[0]), Kind: processor.KindTemplate,
		Config: tmplCfg("{{ .Message.body }}"),
	})
	require.NoError(t, err)
	require.Equal(t, first.Source(first.Outputs[0]), second.InputSource)
}

// TestCreateRejectsUnknownInput: an input naming a source that does not
// exist fails the create rather than producing an inert Processor the
// operator thinks is wired.
func TestCreateRejectsUnknownInput(t *testing.T) {
	ctx := context.Background()
	_, svc := setup(t)

	_, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Bad trigger", InputSource: eventsource.Trigger("s-missing"), Kind: processor.KindTemplate, Config: tmplCfg("x"),
	})
	require.ErrorIs(t, err, store.ErrNotFound)
	require.ErrorContains(t, err, "validate processor input trigger")

	_, err = svc.Create(ctx, org, processors.CreateParams{
		Name: "Bad branch", InputSource: eventsource.ProcessorOutput("p-missing", "po-x"), Kind: processor.KindTemplate, Config: tmplCfg("x"),
	})
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestCreateRejectsCrossTenantInput: a Trigger in another org is not a
// valid input, even though its id exists somewhere.
func TestCreateRejectsCrossTenantInput(t *testing.T) {
	ctx := context.Background()
	s, svc := setup(t)
	orgtest.Trigger(t, s, "org-2", "s-foreign")

	_, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Bad input", InputSource: eventsource.Trigger("s-foreign"), Kind: processor.KindTemplate, Config: tmplCfg("x"),
	})
	require.ErrorIs(t, err, store.ErrNotFound)
	require.ErrorContains(t, err, "validate processor input trigger")
}

// TestDeleteCleansAttachmentsButKeepsHistory: deleting a Processor drops
// every attachment to its branches; the branch streams' events stay.
func TestDeleteCleansAttachments(t *testing.T) {
	ctx := context.Background()
	s, svc := setup(t)
	orgtest.Trigger(t, s, org, "s-in-attached")

	p, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Attached", InputSource: eventsource.Trigger("s-in-attached"), Kind: processor.KindTemplate, Config: tmplCfg("x"),
	})
	require.NoError(t, err)
	node, err := orgchart.NewNode("w-attached", "work", nil, time.Now(), org)
	require.NoError(t, err)
	require.NoError(t, s.Nodes.Create(ctx, node))
	a, err := attachment.New("wa-attached", org, node.ID, p.Source(p.Outputs[0]), "", time.Now())
	require.NoError(t, err)
	require.NoError(t, s.WorkerAttachments.Create(ctx, a))

	// An attached branch cannot be removed on its own.
	err = svc.RemoveOutput(ctx, org, p.ID, p.Outputs[0].ID)
	require.ErrorIs(t, err, store.ErrConflict)
	require.ErrorContains(t, err, "has worker attachments")

	require.NoError(t, svc.Delete(ctx, org, p.ID))
	rows, err := s.WorkerAttachments.Find(ctx, store.WithOrg(org), store.WithProcessorID(p.ID))
	require.NoError(t, err)
	require.Empty(t, rows)
	_, err = svc.Get(ctx, org, p.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestLifecycleMutationsRequireAttachmentRepository(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	svc := processors.New(processors.Deps{Processors: st.Processors, Triggers: st.Triggers, NewID: func() string { return "fixed" }})
	orgtest.Trigger(t, st, org, "s-deps-input")

	p, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Deps", InputSource: eventsource.Trigger("s-deps-input"), Kind: processor.KindTemplate, Config: tmplCfg("x"),
	})
	require.NoError(t, err)
	err = svc.RemoveOutput(ctx, org, p.ID, p.Outputs[0].ID)
	require.ErrorContains(t, err, "attachment repository is not configured")
	err = svc.Delete(ctx, org, p.ID)
	require.ErrorContains(t, err, "attachment repository is not configured")
	_, err = st.Processors.Get(ctx, org, p.ID)
	require.NoError(t, err)
}

func TestUpdateRevalidatesConfig(t *testing.T) {
	ctx := context.Background()
	s, svc := setup(t)
	orgtest.Trigger(t, s, org, "s-in")
	p, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Fmt", InputSource: eventsource.Trigger("s-in"), Kind: processor.KindTemplate,
		Config: tmplCfg("{{ .Message.body }}"),
	})
	require.NoError(t, err)
	// Malformed template rejected at update.
	_, err = svc.Update(ctx, org, p.ID, processors.UpdateParams{
		Name: "Fmt", Kind: processor.KindTemplate, Config: tmplCfg("{{ .Message.body "),
	})
	if err == nil {
		t.Error("want error updating to malformed template, got nil")
	}
}

// TestUpdateDisconnectsInput: a zero input source leaves the Processor
// inert rather than rejecting the update — that is how the chart's
// "delete the input edge" gesture is expressed.
func TestUpdateDisconnectsInput(t *testing.T) {
	ctx := context.Background()
	s, svc := setup(t)
	orgtest.Trigger(t, s, org, "s-in")
	p, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Fmt", InputSource: eventsource.Trigger("s-in"), Kind: processor.KindTemplate,
		Config: tmplCfg("{{ .Message.body }}"),
	})
	require.NoError(t, err)

	disconnected := eventsource.SourceRef{}
	got, err := svc.Update(ctx, org, p.ID, processors.UpdateParams{
		Name: "Fmt", Kind: processor.KindTemplate, Config: tmplCfg("{{ .Message.body }}"),
		InputSource: &disconnected,
	})
	require.NoError(t, err)
	require.True(t, got.InputSource.Zero())
}

func filterRouter(t *testing.T, ctx context.Context, s *store.Store, svc *processors.Processors) processor.Processor {
	t.Helper()
	orgtest.Trigger(t, s, org, "s-slack")
	p, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Router", InputSource: eventsource.Trigger("s-slack"), Kind: processor.KindFilter,
		Outputs: []processors.OutputSpec{{Label: "default"}}, // unconditional default
	})
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	return p
}

func TestAddOutputAllocatesBranchAndPersists(t *testing.T) {
	ctx := context.Background()
	s, svc := setup(t)
	p := filterRouter(t, ctx, s, svc)

	out, err := svc.AddOutput(ctx, org, p.ID, processors.OutputSpec{
		Label: "alice", Match: `{{ mentions "alice" .Message.body }}`, ManagedFor: "w-alice",
	})
	if err != nil {
		t.Fatalf("add output: %v", err)
	}
	if out.ID == "" || out.StreamID == "" || out.ManagedFor != "w-alice" {
		t.Fatalf("unexpected output: %+v", out)
	}
	// Persisted: the processor now has 2 branches.
	got, _ := svc.Get(ctx, org, p.ID)
	if len(got.Outputs) != 2 {
		t.Fatalf("want 2 outputs after add, got %d", len(got.Outputs))
	}
}

func TestRemoveOutputDropsBranch(t *testing.T) {
	ctx := context.Background()
	s, svc := setup(t)
	p := filterRouter(t, ctx, s, svc)
	out, err := svc.AddOutput(ctx, org, p.ID, processors.OutputSpec{Label: "alice", Match: "x", ManagedFor: "w-alice"})
	require.NoError(t, err)

	if err := svc.RemoveOutput(ctx, org, p.ID, out.ID); err != nil {
		t.Fatalf("remove output: %v", err)
	}
	got, _ := svc.Get(ctx, org, p.ID)
	if len(got.Outputs) != 1 {
		t.Fatalf("want 1 output after remove, got %d", len(got.Outputs))
	}
	if _, ok := got.Output(out.ID); ok {
		t.Fatalf("branch %q should be gone", out.ID)
	}
	// Idempotent: removing it again is a no-op.
	require.NoError(t, svc.RemoveOutput(ctx, org, p.ID, out.ID))
}

func TestRemoveLastOutputRejected(t *testing.T) {
	ctx := context.Background()
	s, svc := setup(t)
	p := filterRouter(t, ctx, s, svc)
	// The router starts with exactly one (default) branch; removing it
	// would leave the processor with zero outputs, which Validate forbids.
	if err := svc.RemoveOutput(ctx, org, p.ID, p.Outputs[0].ID); err == nil {
		t.Error("want error removing the last output, got nil")
	}
}

func TestDeleteAutomatedByInputRemovesRouterButNotManual(t *testing.T) {
	ctx := context.Background()
	s, svc := setup(t)
	orgtest.Trigger(t, s, org, "s-ws")
	ws := eventsource.Trigger("s-ws")

	// Automated router on s-ws (CreatedBy = SystemActor).
	auto, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Auto", InputSource: ws, Kind: processor.KindFilter,
		Outputs: []processors.OutputSpec{{Label: "default"}}, CreatedBy: processor.SystemActor,
	})
	if err != nil {
		t.Fatal(err)
	}
	// A human-authored filter on the SAME input — must be left alone.
	manual, err := svc.Create(ctx, org, processors.CreateParams{
		Name: "Manual", InputSource: ws, Kind: processor.KindFilter,
		Outputs: []processors.OutputSpec{{Label: "default"}}, CreatedBy: "w-alice",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteAutomatedByInput(ctx, org, ws); err != nil {
		t.Fatalf("DeleteAutomatedByInput: %v", err)
	}
	if _, err := svc.Get(ctx, org, auto.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("automated router should be deleted, err=%v", err)
	}
	// Manual processor untouched.
	if _, err := svc.Get(ctx, org, manual.ID); err != nil {
		t.Errorf("manual processor should survive, err=%v", err)
	}
	// Idempotent: a second call is a no-op.
	if err := svc.DeleteAutomatedByInput(ctx, org, ws); err != nil {
		t.Errorf("second DeleteAutomatedByInput should be no-op, got %v", err)
	}
}
