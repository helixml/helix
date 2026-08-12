# Org asset MCP management

## Goal

Let the seeded Chief of Staff create and manage org server assets without
requiring the owner to switch to the REST or chart UI. Creating an SSH-key
asset must return everything needed to bootstrap access, while never exposing
Helix's generated private key.

## Tool surface

The Chief of Staff receives these owner-management tools through
`OwnerBotTools`:

- `list_org_assets`, `get_org_asset`
- `create_server_asset`, `update_server_asset`, `delete_asset`
- `list_asset_links`, `link_asset`, `unlink_asset`
- `get_asset_health`

The existing `list_assets`, `get_asset`, and `server_*` tools remain
link-scoped operational capabilities. They are not widened: ordinary agents
must only discover and operate assets explicitly linked to them.

`create_server_asset` links the new asset to the caller in the same action and
can link additional agents through `agent_ids`. If any link fails, creation is
rolled back so the caller does not receive a partially configured result.

For SSH-key authentication the response includes the generated public key and
an idempotent command that installs it in the remote user's
`~/.ssh/authorized_keys`. The private key remains encrypted in Helix and is
never returned. Chief of Staff may run the installation command over normal
SSH when independent access is already available. Otherwise it sends the exact
command to the owner with `ask_human`. It must verify `get_asset_health`, then
exercise a following `server_run_command`, before claiming the asset is ready.

## Boundaries

Tool possession is the org-graph capability boundary, matching the existing
bot/topic/processor/repository mutation tools. The management set is seeded
only onto Chief of Staff, but remains assignable explicitly through the normal
tool-management surface.

All operations derive the organization from the MCP caller. Asset references
resolve only inside that organization. Management projections expose public
connection configuration, links, and public keys, but never encrypted private
keys or passwords.

## Verification

- Unit tests cover create plus caller-link, public-key bootstrap output,
  org-wide discovery, update, link/unlink, health, delete, and recreate.
- Registration tests pin the complete management set in `OwnerBotTools` and
  keep it out of `BaseReadTools`.
- The Chief of Staff seed/backfill path is exercised with the new tools.
- Run targeted org tests and required Go builds.
- In the inner Helix, create an org, activate Chief of Staff, invoke the MCP
  management flow, and verify the following linked operation.
