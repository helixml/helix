// Package briefing holds the pure domain logic that assembles what an
// AI Worker reads at activation time: the per-activation prompt
// (BuildPrompt, rendered from the triggers that woke the Worker) and
// the org-wide worker policy (WorkerPolicy, the canonical
// worker-policy.md). No I/O — it turns domain values (triggers,
// identity, policy text) into the text a runtime feeds to the LLM.
package briefing

import _ "embed"

// WorkerPolicy is the org-wide worker-policy.md text every AI Worker
// reads at the start of every activation. It is fixed across Roles and
// hires — it tells the Worker how to *be* an AI Worker in helix-org, not
// what its job is. Roles cover the latter.
//
// The Helix runtime pushes this to `.context/worker-policy.md` on the
// per-Worker repo's helix-specs branch so every activation can `cat`
// it before deciding what to do.
//
//go:embed worker-policy.md
var WorkerPolicy string
