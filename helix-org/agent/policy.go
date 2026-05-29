// Package agent is the runtime layer that activates AI Workers — the
// thing that takes "this Worker just got an event" and turns it into
// an actual LLM-driven turn. Concrete runtimes live in sub-packages
// (agent/claude, agent/helix). This package holds the runtime-shared
// types: the Spawner contract, Trigger/TriggerKind, the WorkspaceSync
// interface, and the canonical agent-policy text every runtime feeds
// to the LLM.
package agent

import _ "embed"

// Policy is the org-wide agent.md text every AI Worker reads at the
// start of every activation. It is fixed across Roles and hires — it
// tells the agent how to *be* an agent in helix-org, not what its job
// is. Roles cover the latter.
//
// Both runtimes embed this verbatim: the claude runtime writes it as
// `agent.md` in the Worker's env directory, the Helix runtime pushes
// it to `.context/agent.md` on the per-Worker repo's helix-specs
// branch.
//
//go:embed policy.md
var Policy string
