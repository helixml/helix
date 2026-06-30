package transport_test

import (
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// TestKindSpecTask_InKindValues pins that the spec-task transport is a
// registered kind (strategy + kindOrder membership).
func TestKindSpecTask_InKindValues(t *testing.T) {
	t.Parallel()
	found := false
	for _, k := range transport.KindValues() {
		if k == transport.KindSpecTask {
			found = true
		}
	}
	if !found {
		t.Fatalf("KindSpecTask not in KindValues(): %v", transport.KindValues())
	}
}

// TestSpecTaskConfig_RequiresProjectID pins that a spec-task topic must
// name the project whose events it carries.
func TestSpecTaskConfig_RequiresProjectID(t *testing.T) {
	t.Parallel()
	// Missing project_id → invalid.
	bad := transport.Transport{Kind: transport.KindSpecTask, Config: json.RawMessage(`{}`)}
	if err := bad.Validate(); err == nil {
		t.Error("expected validation error for missing project_id")
	}
	// With project_id → valid.
	good := transport.Transport{Kind: transport.KindSpecTask, Config: json.RawMessage(`{"project_id":"prj_01abc"}`)}
	if err := good.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

// TestSpecTaskConfig_Accessor pins the typed accessor returns the project.
func TestSpecTaskConfig_Accessor(t *testing.T) {
	t.Parallel()
	tr := transport.Transport{Kind: transport.KindSpecTask, Config: json.RawMessage(`{"project_id":"prj_01abc"}`)}
	cfg, err := tr.SpecTaskConfig()
	if err != nil {
		t.Fatalf("SpecTaskConfig: %v", err)
	}
	if cfg.ProjectID != "prj_01abc" {
		t.Errorf("ProjectID = %q, want prj_01abc", cfg.ProjectID)
	}
}
