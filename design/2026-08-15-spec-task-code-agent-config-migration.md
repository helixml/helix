# Spec task code-agent config ownership

## Decision

Spec tasks and coding projects own a complete `CodeAgentExecutionConfig`. They no longer depend on a persisted Helix App to resolve runtime, credential type, provider, model, reasoning effort, service tier, or Goose recipe configuration.

Sessions driven by a spec task do not own configuration. Their `ParentApp` and `SessionMetadata.CodeAgentOverrides` remain empty; the task is the single source of truth. General chat and org-agent sessions remain App-backed and may continue to use session overrides.

Org-chart projects are the exception to removing project App links. `Project.DefaultHelixAppID` is also the Bot identity binding for a Worker project, so an `org_agent` App ID remains valid there. Coding projects use `Project.CodeAgentConfig`.

## One-way start migration

Before planning, just-do-it execution, approval/revision continuation, or session resume, the service:

1. Materializes `Project.CodeAgentConfig` from the legacy project App when needed.
2. Clears `Project.DefaultHelixAppID` for coding Apps, but retains it for `org_agent` Apps.
3. Materializes `SpecTask.CodeAgentConfig` from the task App plus `SpecTask.CodeAgentOverrides`, or inherits the project config.
4. Clears `SpecTask.HelixAppID` and `SpecTask.CodeAgentOverrides`.
5. Clears the planning session's `ParentApp` and session overrides and records only the runtime needed for ACP startup.

The migration is idempotent. Legacy columns remain temporarily as read-only migration sources and can be removed in a later schema cleanup after deployed rows have been migrated.

## API boundary

Spec-task create, task update, task execution-config, task-session execution-config, App-based switch, and fork override paths reject App IDs and legacy task overrides. Callers provide `code_agent_config` instead.

Project create/update accepts `code_agent_config` for coding work. A non-empty `default_helix_app_id` is accepted only for an `org_agent` project identity.

## Project task defaults

Project Settings exposes provider ID, model, sandbox runtime, and sandbox size together under **Task Defaults**. Provider and model update `Project.CodeAgentConfig`; environment updates `Project.DefaultSandboxRuntime`; size updates `Project.DefaultSandboxResourceOverrides`.

New tasks snapshot these project values when the create request omits a task-level choice. An explicit task value wins. Legacy projects without a sandbox-size default resolve to 4 vCPU / 8 GB.

## Compatibility

Zed configuration, provider preflight, usage attribution, proxy attribution, and Goose recipe resolution read task/project configs first. Legacy App lookup remains only to let an unmigrated historical task reach the one-way migration boundary.

Helix Apps and agents are otherwise unchanged for org-chart workers and general Helix agent sessions.

## Follow-up removal

After the migration has run across deployed tasks, remove the legacy task columns, legacy fallback reads, hidden request fields, and project coding-App fallback. `DefaultHelixAppID` itself remains for org-agent Worker projects unless that identity relationship receives a separate field.
