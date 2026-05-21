// Package agent is the runtime layer that activates AI Workers — the
// thing that takes "this Worker just got an event" and turns it into
// an actual LLM-driven turn. Concrete runtimes live in sub-packages
// (agent/claude, agent/helix). This package holds the runtime-shared
// types: the Spawner contract and the WorkspaceSync interface, plus
// the canonical agent-policy text every runtime feeds to the LLM.
// Trigger / TriggerKind moved to api/pkg/org/activation in B3c.
package agent

import _ "embed"

// Policy is the org-wide worker-policy.md text every AI Worker reads
// at the start of every activation. It is fixed across Roles and hires
// — it tells the Worker how to *be* an AI Worker in helix-org, not what
// its job is. Roles cover the latter.
//
// Both runtimes embed this verbatim: the claude runtime writes it as
// `worker-policy.md` in the Worker's env directory, the Helix runtime
// pushes it to `.context/worker-policy.md` on the per-Worker repo's
// helix-specs branch.
//
// Naming: see ADR-0001 §2 — "agent" is reserved for the LLM-client-
// binary sense (Claude Code, external-agent runtime). The file used
// to be called `agent.md`.
//
//go:embed worker-policy.md
var Policy string
