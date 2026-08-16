# Organization code-agent policy

Organization settings define two independent inputs to coding tasks:

- Provider endpoints and subscriptions are organization-owned credentials.
- Coding harness policy enables or disables each supported runtime.

Provider endpoints remain the source of available API models. The organization policy does not copy, pin, or rename providers or models. A task selects its harness, credential mode, provider, and model from the current intersection of enabled harnesses and available credentials.

## Data model

`org_code_agent_harnesses` stores one row per organization and runtime:

- `organization_id`
- `runtime`
- `enabled`

There is deliberately no provider reference, credential type, model, default model, or flavour name in this table.

`GET /api/v1/organizations/{org_id}/code-agent-harnesses` returns the policy plus effective Claude and Codex subscription availability for the viewer. `PUT` replaces the enabled state for the submitted runtimes. Organization members may read it; organization owners may update it.

## Enforcement

The API validates the selected harness when a code-agent execution config is created and again before a spec task starts. UI filtering is not an authorization boundary. Personal workspaces retain the existing unrestricted harness list.

Organization provider lookups carry both owner ID and owner type. Treating an organization ID as a user owner makes configured org providers invisible, so provider-manager calls must not infer or omit ownership type on org-scoped paths.

## Task selection

The task chat control:

1. filters the harness rail by organization policy;
2. lists models directly from currently visible provider endpoints;
3. offers subscription or API credentials for Claude Code and Codex when available;
4. stores the selected provider by immutable provider reference and the selected model on the task config.

Changing organization policy affects future selections and blocks disabled stored configs from executing. It does not rewrite existing task configs.
