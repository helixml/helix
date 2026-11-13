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
| `api` | API key | `api_key_owner`, `api_key_type` (scope: personal, org, project, spec_task, session, sandbox, app, embed, bot_instance), `api_key_id` (the non-secret `key_…` id from `types.APIKeyID`), `api_key_org_id` / `_project_id` / `_spec_task_id` / `_session_id` |
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
