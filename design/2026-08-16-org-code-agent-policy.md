# Organization code-agent policy

Organization settings define two layers of coding-task policy:

- Coding harness policy enables or disables each supported runtime.
- Each enabled harness has an allow-list of usable credential sources: API
  provider endpoints and, where supported, members' connected subscriptions.

Provider endpoints and subscription catalogues remain the sources of available models. The organization policy does not copy, pin, or rename models. A task selects a harness and model from the current intersection of enabled harnesses, enabled credential sources, and credentials available to that member. The selected model option carries its credential type and provider reference; the task picker does not expose a separate credential-mode control.

## Data model

`org_code_agent_harnesses` stores one row per organization and runtime:

- `organization_id`
- `runtime`
- `enabled`
- `subscription_enabled` (nullable for migration compatibility)
- `provider_refs` (nullable JSON array of immutable endpoint references)

There is deliberately no model, default model, or flavour name in this table. A null source policy preserves the pre-policy behavior for existing rows: supported subscriptions and all visible providers remain enabled. Once an owner changes a source switch, the explicit value is persisted; an empty provider list means no API providers are exposed to that harness.

`GET /api/v1/organizations/{org_id}/code-agent-harnesses` returns the policy plus effective Claude and Codex subscription availability for the viewer. `PUT` updates submitted harness rows while preserving nullable source fields omitted by the caller. Organization members may read it; organization owners may update it.

## Enforcement

The API validates the selected harness and credential source when a code-agent execution config is created, before a spec task starts, and when a sandbox fetches its Zed configuration. Provider snapshots used to generate Zed's built-in model list are filtered by the harness allow-list. UI filtering is not an authorization boundary. Personal workspaces retain the existing unrestricted behavior.

Organization provider lookups carry both owner ID and owner type. Treating an organization ID as a user owner makes configured org providers invisible, so provider-manager calls must not infer or omit ownership type on org-scoped paths.

## Task selection

The task chat control:

1. filters the harness rail by organization policy;
2. combines models from the subscription and API sources enabled for that harness;
3. omits credential-mode controls because source eligibility is configured in organization settings;
4. stores the model option's credential type, immutable provider reference where applicable, and model on the task config.

When a new task has no valid execution configuration, the composer selects an
enabled subscription-backed harness automatically. Claude Code defaults to
Claude Opus 5 and Codex defaults to GPT-5.6 Sol; if both are available, Claude
Code wins the deterministic default order. This is task initialization, not
organization model policy, and the user can change it in the task chat.

Changing organization policy affects future selections and blocks disabled stored configs from executing. It does not rewrite existing task configs.
