# GPU & inference onboarding flow

**Status:** design / not started.
**Owner:** unassigned.
**Triggered by:** post sandbox-absorbs-runner pivot, the deleted
`runnerController.RunnerIDs()` gate that used to hide the Helix provider
when no runners were attached has been replaced by an
`inferenceRouter.AvailableModels()` gate (commit on this branch). That
gate hides the provider but does nothing to *help* an operator get
inference running. A first-time admin lands on a blank dashboard with
no obvious next step.

## Problem

Today, after a fresh install:

- `GET /v1/models` returns an empty list.
- The Helix provider is correctly hidden from the picker (post-fix).
- The Runner Profiles page lists curated templates but the operator
  has no sandbox to assign them to.
- The Agent Sandboxes page is empty.
- There is no signposting for "how do I get a GPU attached so I can
  serve a model."

The operator has to (a) read docs to discover that they need to either
provision a cloud GPU or run the sandbox bootstrap on an on-prem host,
(b) install the sandbox, (c) wait for it to register, (d) assign a
profile, (e) wait for the model to download (now visible thanks to the
download-progress bars). Steps (a) and (b) are entirely
documentation-shaped; the UI doesn't help.

## Proposed flow

A wizard that fires when **all** of:

1. The current org has zero registered sandboxes (`store.ListSandboxes()` empty).
2. The current org has zero non-Helix providers configured (`providerManager.ListProviders(orgID)` empty).
3. The user is an org admin (the wizard is gated to roles that can
   actually act on it; viewers see a static "ask an admin to attach a
   GPU" empty state).

Wizard steps:

1. **Welcome** — "Helix needs a GPU to serve models. Two paths:"
   - **Cloud** — provision via Verda (NVIDIA) or Hot Aisle (AMD). One
     form: provider, instance SKU, region. Submit triggers the
     existing `internal/provision/` path. Show the live provisioning
     log (already structured) until the sandbox heartbeats in.
   - **On-prem** — show the `sandbox-bootstrap` install one-liner with
     a freshly-minted runner token, and a polling indicator that
     turns green when the heartbeat arrives.
   - **Skip** — for users who only want third-party providers
     (OpenAI/Anthropic). Drops them on the Providers page.
2. **Pick a profile** — render the existing Profile Gallery,
   pre-filtered to profiles that match the new sandbox's GPU
   inventory (vendor + arch + min-VRAM compatibility check). Operator
   picks one; we POST the assignment.
3. **Wait for model download** — show the per-service progress bars
   from the AgentSandboxes change just landed. When ProfileStatus
   transitions to `running`, advance.
4. **Try it** — drop the operator into a chat playground with the
   served model preselected. First prompt confirms the loop.

Skip affordances at every step. The wizard should also be
re-launchable from a "Add another sandbox" button on the dashboard,
not only on first install.

## Why this is worth doing

- The pivot removed scheduler complexity but lost the
  "runner-management" surface that previously *implicitly* told
  operators what to do. Without onboarding, the empty state is a
  blocker that no amount of inline docs cures — operators bounce.
- The provisioning code, the profile gallery, the GPU compatibility
  check, the download-progress bars: every component the wizard
  needs already exists. This is a glue/UX project, not new systems.

## Why it's *not* in this branch

- The trigger conditions need careful thought (org-scoped vs system,
  RBAC, multi-tenant SaaS where the trigger never fires for
  customers). Wrong triggers are worse than no wizard.
- The Cloud path needs a billing/cost-confirmation modal before
  spend is incurred. Out of scope for an in-flight tweak.
- ~3-5 days of focused work, not a side-quest in a refactor PR.

## Spec needed before building

1. Trigger conditions and "skip wizard" affordances (decision: is
   this org-level, user-level, or system-level state?).
2. Cost confirmation copy + threshold for cloud GPU provisioning.
3. Behaviour when provisioning fails partway (timeout, no stock,
   API error). The wizard must surface this and offer a retry path.
4. Test plan: how do we drive this in CI without burning real GPU
   spend? Suggest a `MOCK_PROVISIONING=true` env that returns canned
   sandbox heartbeats so the UI flow can be exercised.

## Related

- `api/pkg/server/provider_handlers.go` — gate that hides Helix
  provider when no models are available.
- `internal/provision/` — Verda + Hot Aisle clients, ready to drive.
- `frontend/src/components/dashboard/ProfileGallery.tsx` — picker UI
  to reuse in step 2.
- `frontend/src/components/admin/AgentSandboxes.tsx` — progress UI to
  reuse in step 3 (download bars added in `b1e9cbfdb`).
