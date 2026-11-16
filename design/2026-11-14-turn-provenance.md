# Turn provenance

Every user-role turn (an interaction's prompt, a queued prompt-history entry)
records who caused it, so an external verifier reading
`GET /api/v1/sessions/{id}/interactions` can tell a person's message from an
API caller's and from Helix's own templated prompts.

## Shape

`types.TurnProvenance`, stored as a `jsonb` `provenance` column on
`interactions` and `prompt_history_entries`:

| origin | meaning | fields |
|---|---|---|
| `user` | browser session (OIDC or password) | `user_id` |
| `api` | API key | `api_key_owner`, `api_key_type` (scope: personal, org, project, spec_task, session, sandbox, app, embed, bot_instance; a task-confined key is `spec_task`), `api_key_id` (the non-secret `key_…` id from `types.APIKeyID`), `api_key_org_id` / `_project_id` / `_spec_task_id` / `_session_id`, `api_key_task_confined` |
| `system` | Helix itself | `system_kind` (`types.SystemTurnKind`), `org_bot_id` for org-bot turns, `triggered_by` (the caller's own provenance) when an authenticated request made Helix write it |
| `external_agent` | typed into, or a thread opened in, the agent's Zed desktop; Helix cannot tell who typed it | — |
| `unknown` | row written before the column existed (NULL); never written by a creation path | — |

## Rules

- Set only server-side at creation: from the authenticated request user
  (`types.RequestTurnProvenance`), from the system call site, or from an
  in-process caller via `withSystemTurn` (an unexported context key). No
  request body type carries a provenance field
  (`TestTurnRequestBodiesHaveNoProvenanceField`).
- Credentials that cannot send turns (runner, socket, anonymous) are refused.
- Immutable: `UpdateInteraction` omits the column. A prompt-sync that rewrites a
  queued entry's content restamps it with the editor's provenance.
- The enqueue helpers, `SpecTaskMessageEnqueuer`, `controller.RunSessionRequest`
  and `SyncPromptHistory` all take provenance and reject an invalid one.
- `pkg/types/turn_provenance_sites_test.go` fails when a turn-creating literal
  omits `Provenance`, or when a system kind appears at an unregistered call site.

## Deferred work keeps its caller

A `system` turn is never anonymous when a caller asked for it, even if Helix
sends it much later. The caller's provenance is captured at request time and
stored with the queued work, and the eventual turn is
`system` + kind + `triggered_by` that caller:

| Deferred work | Stored on | Kinds |
|---|---|---|
| Design-review comment | `spec_task_design_review_comments.provenance` | `design_review_comment` |
| Agent work the orchestrator starts later: task creation, start planning, approve or reject specs | `spec_tasks.queued_by` | `planning_prompt`, `implementation_prompt`, `revision_requested`, phase `agent_switched` / `agent_switch_handoff` |
| Pull request outcome sent once commits land | `spec_task_pr_proposals.decider` | `pr_proposal_outcome` |
| Attachment upload, agent switch, evaluation run, manual cron run | passed from the request | `attachment_added`, `agent_switched`, `agent_switch_handoff`, `evaluation_question`, `cron_trigger` |

These columns are `json:"-"`, so no request body can set them.
`pkg/types/turn_provenance_sites_test.go` fails when a function queues agent
work on a task (a queued status or a spec approval) without setting `QueuedBy`,
or when a literal for any of these carriers omits the caller.

Non-human callers are named by actor kinds that only appear inside
`triggered_by`: `org_bot_action` (with `org_bot_id`), `helix_agent_tool`,
`pr_auto_approve`, and `trigger_configuration` (with `trigger_id`,
`trigger_app_id`, `trigger_owner`). Slack, Teams, Crisp, Azure DevOps, cron and
code-review turns name their trigger configuration this way. The external
sender (a Slack user, a GitHub reviewer) is not known to Helix and is not
recorded. Rows from before a carrier existed give `triggered_by: {"origin": "unknown"}`.
