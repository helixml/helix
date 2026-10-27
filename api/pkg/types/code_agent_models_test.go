package types

import "testing"

// Every list must lead with a current model (the default), keep current models
// ahead of legacy ones, and hold no duplicates, or the pickers and the default
// disagree about what "best" is.
func TestCodeAgentModelsAreOrderedBestFirst(t *testing.T) {
	for runtime, models := range CodeAgentModels {
		if len(models) == 0 {
			t.Fatalf("%s: empty model list", runtime)
		}
		if got := DefaultCodeAgentModel(runtime); got != models[0].ID || !models[0].Current {
			t.Errorf("%s: default %q must be the first entry %q and current", runtime, got, models[0].ID)
		}
		seen := map[string]bool{}
		legacy := false
		for _, m := range models {
			if seen[m.ID] {
				t.Errorf("%s: duplicate model %q", runtime, m.ID)
			}
			seen[m.ID] = true
			if !m.Current {
				legacy = true
			} else if legacy {
				t.Errorf("%s: current model %q listed after a legacy one", runtime, m.ID)
			}
		}
	}
}

func TestDefaultCodeAgentModelForRuntimeWithoutAList(t *testing.T) {
	if got := DefaultCodeAgentModel(CodeAgentRuntimeOpenCode); got != "" {
		t.Fatalf("opencode default = %q, want empty: it runs whatever provider the operator configured", got)
	}
}
