package mcptools

import (
	"reflect"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// TestBaseReadToolsGolden pins the exact contents of the universal
// read baseline. Anyone adding/removing entries has to update this
// list in the same commit — forcing a deliberate review of what every
// Bot in the system will expose by default.
func TestBaseReadToolsGolden(t *testing.T) {
	t.Parallel()
	want := []tool.Name{
		ManagersName,
		ReportsName,
		ListBotsName,
		GetBotName,
		ListTriggersName,
		GetTriggerName,
		ListTriggerEventsName,
		ReadEventsName,
		BotLogName,
		GetSecretName,
		ListSecretsName,
		ListProcessorsName,
		GetProcessorName,
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
		ManagersName:          &Managers{deps: deps},
		ReportsName:           &Reports{deps: deps},
		ListBotsName:          &ListBots{deps: deps},
		GetBotName:            &GetBot{deps: deps},
		ListTriggersName:      &ListTriggers{deps: deps},
		GetTriggerName:        &GetTrigger{deps: deps},
		ListTriggerEventsName: &ListTriggerEvents{deps: deps},
		ReadEventsName:        &ReadEvents{deps: deps},
		BotLogName:            &BotLog{deps: deps},
		GetSecretName:         &GetSecret{deps: deps},
		ListSecretsName:       &ListSecrets{deps: deps},
		ListProcessorsName:    &ListProcessors{deps: deps},
		GetProcessorName:      &GetProcessor{deps: deps},
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
	in := []tool.Name{ChatName, ManagersName, ChatName}
	got := MergeBaseReadTools(in)

	// Expected order: caller's deduped order first, then baseline
	// names not yet present in baseline order (managers is skipped).
	want := []tool.Name{
		ChatName,
		ManagersName,
		// rest of baseline, in BaseReadTools order, minus managers:
		ReportsName,
		ListBotsName,
		GetBotName,
		ListTriggersName,
		GetTriggerName,
		ListTriggerEventsName,
		ReadEventsName,
		BotLogName,
		GetSecretName,
		ListSecretsName,
		ListProcessorsName,
		GetProcessorName,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge drifted.\n got: %v\nwant: %v", got, want)
	}
}

// TestMergeBaseReadToolsIdempotent ensures a second pass on an
// already-merged list is a no-op. The reconciler relies on this for
// idempotency — if merge re-ordered the list on every call, Reconcile
// would rewrite every Bot on every run.
func TestMergeBaseReadToolsIdempotent(t *testing.T) {
	t.Parallel()
	in := []tool.Name{ChatName, DMName}
	once := MergeBaseReadTools(in)
	twice := MergeBaseReadTools(once)
	if !reflect.DeepEqual(once, twice) {
		t.Fatalf("merge is not idempotent.\n once: %v\ntwice: %v", once, twice)
	}
}

// TestDefaultBotToolsGolden pins the operational surface granted to every
// newly created standard Bot. Organization-management mutations must remain
// an explicit manager capability.
func TestDefaultBotToolsGolden(t *testing.T) {
	t.Parallel()
	want := []tool.Name{
		ChatName,
		DMName,
		ListProjectsName,
		GetProjectName,
		ListRepositoriesName,
		ListBotRepositoriesName,
		ListAssetsName,
		GetAssetName,
		CreateSpecTaskName,
		ListSpecTasksName,
		GetSpecTaskName,
		UpdateSpecTaskName,
		StartSpecTaskPlanningName,
		SendSpecTaskAgentMessageName,
		ListSpecTaskAgentMessagesName,
		StartSpecTaskAgentName,
		StopSpecTaskAgentName,
		RestartSpecTaskAgentName,
		ReviewSpecTaskSpecName,
		ApproveSpecTaskSpecName,
		RequestSpecTaskChangesName,
		CreateSpecTaskPRsName,
		ManagersName,
		ReportsName,
		ListBotsName,
		GetBotName,
		ListTriggersName,
		GetTriggerName,
		ListTriggerEventsName,
		ReadEventsName,
		BotLogName,
		GetSecretName,
		ListSecretsName,
		ListProcessorsName,
		GetProcessorName,
	}
	if got := DefaultBotTools(); !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultBotTools drifted from golden list.\n got: %v\nwant: %v", got, want)
	}
}

func TestMergeDefaultBotToolsPreservesAdditionsAndDedups(t *testing.T) {
	t.Parallel()
	in := []tool.Name{AttachWorkerName, ChatName, AttachWorkerName}
	got := MergeDefaultBotTools(in)
	want := append([]tool.Name{AttachWorkerName}, DefaultBotTools()...)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default merge drifted.\n got: %v\nwant: %v", got, want)
	}
}

func TestHasNonDefaultBotTool(t *testing.T) {
	t.Parallel()
	if HasNonDefaultBotTool(DefaultBotTools()) {
		t.Fatal("standard worker tools must not require organization-manager access")
	}
	if !HasNonDefaultBotTool([]tool.Name{ChatName, CreateBotName}) {
		t.Fatal("create_bot must require organization-manager access")
	}
}

func TestOwnerBotToolsContainsStandardAndManagementCapabilities(t *testing.T) {
	t.Parallel()
	got := OwnerBotTools()
	counts := make(map[tool.Name]int, len(got))
	for _, name := range got {
		counts[name]++
	}
	for _, name := range DefaultBotTools() {
		if counts[name] != 1 {
			t.Errorf("standard tool %q appears %d times in owner set", name, counts[name])
		}
	}
	for _, name := range []tool.Name{CreateBotName, AttachToolName, AttachRepositoryName, CreateSandboxName} {
		if counts[name] != 1 {
			t.Errorf("management tool %q appears %d times in owner set", name, counts[name])
		}
	}
}
