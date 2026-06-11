package tools

import (
	"reflect"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// TestBaseReadToolsGolden pins the exact contents of the universal
// read baseline. Anyone adding/removing entries has to update this
// list in the same commit — forcing a deliberate review of what every
// Role in the system will expose by default.
func TestBaseReadToolsGolden(t *testing.T) {
	t.Parallel()
	want := []tool.Name{
		ManagersName,
		ReportsName,
		ListWorkersName,
		GetWorkerName,
		ListRolesName,
		GetRoleName,
		ListStreamsName,
		GetStreamName,
		ListStreamEventsName,
		ReadEventsName,
		WorkerLogName,
		GetWorkerEnvironmentName,
		MintCredentialName,
	}
	if !reflect.DeepEqual(BaseReadTools, want) {
		t.Fatalf("BaseReadTools drifted from golden list.\n got: %v\nwant: %v", BaseReadTools, want)
	}
}

// TestBaseReadToolsAllRegistered guarantees every name in BaseReadTools
// resolves in the registry. RegisterBuiltins enforces the same invariant
// at process start; this test gives a much faster signal during local
// development.
func TestBaseReadToolsAllRegistered(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	deps := Deps{}
	// We can't call RegisterBuiltins (it requires non-nil deps fields),
	// so we register each baseline tool's struct individually. The
	// mapping below has to stay in sync with builtins.go; if a name is
	// added to BaseReadTools without a matching entry here, the test
	// fails — same failure mode RegisterBuiltins would produce at boot.
	baselineImpls := map[tool.Name]tool.Tool{
		ManagersName:             &Managers{deps: deps},
		ReportsName:              &Reports{deps: deps},
		ListWorkersName:          &ListWorkers{deps: deps},
		GetWorkerName:            &GetWorker{deps: deps},
		ListRolesName:            &ListRoles{deps: deps},
		GetRoleName:              &GetRole{deps: deps},
		ListStreamsName:          &ListStreams{deps: deps},
		GetStreamName:            &GetStream{deps: deps},
		ListStreamEventsName:     &ListStreamEvents{deps: deps},
		ReadEventsName:           &ReadEvents{deps: deps},
		WorkerLogName:            &WorkerLog{deps: deps},
		GetWorkerEnvironmentName: &GetWorkerEnvironment{deps: deps},
		MintCredentialName:       &MintCredential{deps: deps},
	}
	for _, name := range BaseReadTools {
		impl, ok := baselineImpls[name]
		if !ok {
			t.Fatalf("BaseReadTools name %q has no impl in the test mapping — update the test", name)
		}
		if err := reg.Register(impl); err != nil {
			t.Fatalf("register %q: %v", name, err)
		}
	}
	for _, name := range BaseReadTools {
		if _, err := reg.Get(name); err != nil {
			t.Fatalf("registry lookup for baseline tool %q failed: %v", name, err)
		}
	}
}

func TestMergeBaseReadToolsEmptyInput(t *testing.T) {
	t.Parallel()
	got := MergeBaseReadTools(nil)
	if !reflect.DeepEqual(got, BaseReadTools) {
		t.Fatalf("empty input should return BaseReadTools verbatim.\n got: %v\nwant: %v", got, BaseReadTools)
	}
}

func TestMergeBaseReadToolsPreservesCallerOrderAndDedups(t *testing.T) {
	t.Parallel()
	// Caller-supplied: includes one baseline name (managers) and one
	// non-baseline mutation (publish), with a duplicate to verify dedup.
	in := []tool.Name{PublishName, ManagersName, PublishName}
	got := MergeBaseReadTools(in)

	// Expected order: caller's deduped order first, then baseline
	// names not yet present in baseline order (managers is skipped).
	want := []tool.Name{
		PublishName,
		ManagersName,
		// rest of baseline, in BaseReadTools order, minus managers:
		ReportsName,
		ListWorkersName,
		GetWorkerName,
		ListRolesName,
		GetRoleName,
		ListStreamsName,
		GetStreamName,
		ListStreamEventsName,
		ReadEventsName,
		WorkerLogName,
		GetWorkerEnvironmentName,
		MintCredentialName,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge drifted.\n got: %v\nwant: %v", got, want)
	}
}

// TestMergeBaseReadToolsIdempotent ensures a second pass on an
// already-merged list is a no-op. The reconciler relies on this for
// idempotency — if merge re-ordered the list on every call, Reconcile
// would rewrite every Role on every run.
func TestMergeBaseReadToolsIdempotent(t *testing.T) {
	t.Parallel()
	in := []tool.Name{PublishName, DMName}
	once := MergeBaseReadTools(in)
	twice := MergeBaseReadTools(once)
	if !reflect.DeepEqual(once, twice) {
		t.Fatalf("merge is not idempotent.\n once: %v\ntwice: %v", once, twice)
	}
}
