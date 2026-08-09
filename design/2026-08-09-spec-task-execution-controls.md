# Spec task execution controls

Spec tasks own their coding-agent and sandbox choices. Because a task has one durable
session and one branch, the selected configuration is stored on `SpecTask`; it is not
duplicated into session metadata.

## Persisted configuration

- `helix_app_id` selects the coding-agent app.
- `code_agent_overrides` selects provider, model, reasoning effort, and service tier.
- `sandbox_resource_overrides` selects one supported CPU/memory preset.

Task creation accepts all three values. The task execution-config endpoint replaces
either the coding configuration or sandbox preset atomically.

## Runtime changes

Sandbox containers are resized in place through Docker's container-update API. The
supported presets are 1 vCPU/2 GB, 4 vCPU/8 GB, and 8 vCPU/16 GB. New tasks default to
4 vCPU/8 GB; a missing override on an older task means uncapped resources.

Coding-agent changes keep the task, branch, files, sandbox, and Helix session. They
start a new ACP-native thread and seed it with normalized readable history. The UI
confirms this before a running-task change because agent-native state and provider
prompt-cache entries do not transfer. For a same-agent model switch, the warning names
the old and new models explicitly.

## UI

The task composer and task detail view share one execution control:

- a two-pane coding-agent/model picker;
- a combined reasoning-effort/service-tier menu;
- a CPU control that always displays the effective vCPU count.

The same values are available in chat-first task creation and the full new-task form.
