// Package agent holds the runtime-shared text every AI Worker runtime
// feeds to the LLM at activation time — currently just the canonical
// worker-policy.md. The only production runtime lives at
// api/pkg/org/runtime/helix/; it embeds Policy and pushes it to
// `.context/worker-policy.md` on the per-Worker repo's helix-specs
// branch. The Spawner / WorkspaceSync ports moved to
// api/pkg/org/runtime in B3d; Trigger / TriggerKind moved to
// api/pkg/org/activation in B3c; the dev-only claude-subprocess
// runtime was deleted in B9.
package agent

import _ "embed"

// Policy is the org-wide worker-policy.md text every AI Worker reads
// at the start of every activation. It is fixed across Roles and hires
// — it tells the Worker how to *be* an AI Worker in helix-org, not what
// its job is. Roles cover the latter.
//
// The Helix runtime pushes this to `.context/worker-policy.md` on the
// per-Worker repo's helix-specs branch so every activation can `cat`
// it before deciding what to do.
//
// Naming: see ADR-0001 §2 — "agent" is reserved for the LLM-client-
// binary sense (Claude Code, external-agent runtime). The file used
// to be called `agent.md`.
//
//go:embed worker-policy.md
var Policy string
