# Unify Helix Org Bots with Agents

**Date:** 2026-07-27
**Status:** Implemented, pending review and connected-Zed verification

## Summary

- `types.App` is the canonical execution configuration for an Agent.
- `org_bots` remains the org-graph membership row and links directly to its Agent App through `agent_app_id`.
- The UI and CLI use "Agent"; old Bot URLs and commands remain compatibility aliases.

## Decision

An Agent has one execution configuration and may also participate in a Helix
Org graph.

`types.App` owns the execution configuration:

- display name and system prompt
- runtime, provider, model, credentials, and reasoning settings
- the single Assistant used by the org runtime

`org_bots` owns graph state:

- stable graph handle
- reporting lines
- org MCP tool grants and subscriptions
- project associations, identity, and context-preservation policy
- the direct `agent_app_id` association

Org-linked Agent Apps must contain exactly one Assistant. Human placeholders
remain graph-only rows and do not have an Agent App.

The existing `name` and `content` columns remain on `org_bots` for compatibility
with legacy rows and internal graph APIs. Linked Agents are read from
`types.App`, and writes update the App through the org endpoint. Removing the
legacy columns and renaming internal Bot types is intentionally out of scope for
this migration.

## Data migration and invariants

Startup migration adds nullable `org_bots.agent_app_id`.

Existing links are backfilled from `org_bot_runtime_state` only when:

- the runtime value references an existing same-org App with exactly one
  Assistant
- the row is an Agent, not a human placeholder
- no second Bot claims the same runtime App

A partial unique index prevents two rows in one org from linking the same App.
A foreign key requires the App to exist and uses `ON DELETE RESTRICT`, so the
graph row must be removed before its App.

After org seeding, reconciliation creates an Agent App for each non-human legacy
row that still has no link, then writes `agent_app_id`. New Agent row creation
is rejected by the repository when no App link is present. Human row creation
is rejected when an App link is present.

Startup then converges each non-deleted same-org project associated with exactly
one Bot. A valid, same-org, one-Assistant project default that is not claimed by
another Bot has precedence: it is preserved and becomes the canonical App.
Otherwise the migration uses the valid Bot App, including a link adopted from
runtime state. The chosen App is written to the project default, Bot link, and
runtime link in one database transaction. Projects associated with multiple
Bots are ambiguous and skipped.

Convergence is idempotent. The first org bootstrap creates Apps for remaining
unlinked non-human rows; repeated startup and bootstrap runs leave established
links unchanged. Missing, invalid, cross-org, and ambiguous candidates are not
adopted. The migration does not recreate the project, repository, or session,
so their IDs remain stable.

The runtime project path keeps the same in-place update as a safety fallback
for rows skipped or missed at startup. Activation is therefore not required for
eligible projects to converge, and the fallback does not create a replacement
project.

The service layer verifies that linked Apps belong to the requested org when
applying projects. The database foreign key verifies existence, not matching
organization ownership.

## Lifecycle behavior

### Create

1. Create the canonical Agent App.
2. Create the graph row with `agent_app_id`.
3. Apply reporting lines, subscriptions, and optional activation.
4. Run the shared delete lifecycle if any fatal post-create step fails.

Project application reuses the linked App as `DefaultHelixAppID`; it no longer
creates a second App for the org node. Runtime prompt construction reads the
linked App's Assistant system prompt.

When the org default Agent configuration is already complete, creation resolves
the runtime, credential type, provider, model, and reasoning effort into the App
before it is persisted, then activates the Agent immediately. Runtime and
credential compatibility is normalized as part of that resolution.

When the org default Agent configuration is not complete, creation still
persists the App and graph row but defers project provisioning and activation.
Once the configuration becomes complete, the settings flow applies the resolved
defaults only if the App is still the untouched deferred scaffold, then
activates it. User-edited Apps and already provisioned Apps are not overwritten
by later org default changes.

Creation is not one database transaction. A graph-row creation failure deletes
the new App directly. A later reporting, topology, subscription, hire-hook, or
activation-row failure runs the full delete lifecycle; a rollback failure is
returned with the original error.

### Update

The org detail page sends one PATCH containing graph fields and the Agent App
configuration. The server updates graph state and the canonical App in one user
operation. If the App update fails, it restores the previous graph fields and
returns an error.

The MCP `set_bot_content` tool also updates the linked App's system prompt. If
the canonical update is unavailable or fails, it restores the previous graph
content and does not mirror the failed value into the running workspace.

Updates through the standard Agent endpoint detect linked Apps, require exactly
one Assistant, and normalize the Assistant name to the App name. This prevents
the generic App surface from reintroducing a second name or an unsupported
multi-Assistant shape.

List and detail responses read name and instructions from the linked App, so
changes made through the standard Agent surface appear in the org surface.

### Delete

Deleting an org Agent tears down its project first. The Helix adapter resolves
the project's organization owner and executes project deletion as that owner.
An already missing project is accepted so an interrupted deletion can be
retried safely.

After project teardown, one shared database transaction deletes the Agent's
knowledge versions and knowledge, runtime state, subscriptions, graph row, and
canonical App. Reporting lines cascade with graph-row deletion. Any failure
rolls back the whole transaction, retaining the graph row and App. A retry can
then complete the transaction even when the first attempt already deleted the
project.

Deleting the linked App through the standard Agent endpoint detects the org
link and delegates to the same lifecycle, so the org chart cannot retain a
dangling node. `keep_knowledge=true` is rejected for an org-linked Agent because
the shared lifecycle performs full App cleanup.

## UI and compatibility

The Helix Org navigation, list, creation dialog, detail page, notifications,
and delete confirmation use "Agent".

Canonical routes are:

```text
/orgs/:org_id/helix-org/agents
/orgs/:org_id/helix-org/agents/:bot_id
```

The previous `/helix-org/bots` routes redirect to the Agent routes.

REST exposes `/agents` as the canonical flat resource. `GET /agents` returns
flat Agent rows, and `GET /agents/{id}` returns the same configuration fields
plus `project_id`; PATCH writes those flat fields. The compatibility `/bots`
routes remain available, including the legacy nested detail response expected
by older clients. The CLI command is `helix org agents`; `agent`, `bots`, and
`bot` remain aliases.

MCP tool names such as `create_bot`, `list_bots`, and `delete_bot` are
intentionally unchanged. Renaming or duplicating them would expand the MCP
capability surface and break stored tool allowlists and prompts. MCP naming can
be migrated separately only with an explicit versioning and allowlist plan.

## Verification

Live create, list, rename, delete, and recreate passed against the implemented
API.

Live Zed activation provisioned the Agent's project, repository, and session
row. The desktop Agent could not connect because the isolated alternate API
used for the test had no Hydra RevDial connection. The connected-Zed message
and thread-switch seam therefore remains unverified.

## Rollout and verification risks

- Run the migration against a Postgres copy containing valid, dangling,
  duplicate, cross-org, soft-deleted, and conflicting runtime links. Verify
  eligible project defaults change in place while skipped project, repository,
  session, and runtime IDs remain unchanged. Ambiguous Bot links deliberately
  remain unlinked and reconciliation creates new Apps; old runtime Apps may
  then require orphan cleanup.
- Upgrade every backend API instance together before publishing the new
  frontend. Mixed-version API deployment and downgrade are unsupported for
  this migration because old instances use App-first deletion while the new
  schema enforces graph-before-App deletion with `ON DELETE RESTRICT`.
- Inject failures after graph-row creation, during App update, and during App
  deletion to verify compensation and surfaced errors.
- Verify create, edit, and delete from both the Helix Org Agent page and the
  standard Agent page. Confirm both surfaces show the same name, instructions,
  runtime, and model after each edit.
- Verify old UI URLs, REST `/bots` calls, and CLI `bots` commands still work.
- Before activating a migrated Agent, confirm startup already updated its
  existing project's default App without changing project, repository, session,
  or runtime IDs. Then activate it and confirm the runtime fallback leaves the
  same canonical `agent_app_id` in place.
- Repeat the activation test against an API with a working Hydra RevDial
  connection. Update the runtime, switch the active session, send the next
  message, and verify no duplicate App or unintended thread switch is created.
