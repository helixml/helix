# Org assets

## Goal

Add typed, first-class assets to the Helix org graph. The first asset type is
`server`: a user registers an SSH endpoint, installs the Helix-generated public
key on it, links it to selected agents, and those agents receive server command
and file tools. Helix also exposes an SSH proxy for interactive access without
distributing the server's private connection details to agent sandboxes.

## Non-goals for the first slice

- Generic arbitrary asset schemas configured at runtime.
- Windows/WinRM servers.
- Treating a successful ICMP echo as proof that SSH is usable.

## Domain model

`asset.Asset` is the aggregate and owns one typed payload. GORM persists the
payload as JSON through `serializer:json`, allowing new asset payload structs
without adding nullable columns for every type.

```go
type Asset struct {
    ID, OrganizationID, Name, Description, NotesForAgents string
    Kind AssetKind
    Config Config
    CreatedAt, UpdatedAt time.Time
}

type Config struct {
    Server *Server `json:"server,omitempty"`
}

type Server struct {
    Address             string `json:"address"`
    Port                uint16 `json:"port"`
    User                string `json:"user"`
    AuthType            string `json:"auth_type"`
    PublicKey           string `json:"public_key"`
    EncryptedPrivateKey string `json:"encrypted_private_key"`
    EncryptedPassword   string `json:"encrypted_password,omitempty"`
    HostKey             string `json:"host_key,omitempty"`
}
```

Exactly one payload matching `Kind` must be present. The REST DTO never exposes
`EncryptedPrivateKey`. The key is Ed25519 and encrypted with the Helix encryption
key before persistence. `HostKey` pins the SSH host key; Helix captures it on the
first reachable connection and rejects changes thereafter.

`asset.Link` is the org-scoped `(asset_id, agent_id)` capability edge. Creating a
server link attaches the server tool set to `Node.Tools`; removing the final
server link removes that derived tool set. Each tool still verifies the live
link, so a stale or manually attached tool name cannot bypass authorization.

## Server operations

The API and MCP surface mirror Helix's existing Sandboxes API and Vercel
Sandbox's current model:

- run a command with `cmd`, `args`, `cwd`, `env`, `sudo`, `detached`, and timeout;
- list/get commands and kill a detached command;
- list/read/write files, including write mode;
- report TCP reachability separately from authenticated SSH reachability.

Detached command metadata and the live SSH session are held by the API process.
This is operational state, not org-graph state. The process may continue on the
server if the API restarts, but its Helix command handle is no longer listable.
Helix does not pretend it can rediscover arbitrary remote processes as its own.

The command shape follows the 2026 Vercel Sandbox API: detached execution,
command listing/status/kill, and explicit file read/write operations. Relevant
primary references:

- <https://vercel.com/docs/sandbox>
- <https://vercel.com/kb/guide/vercel-sandbox-duration-and-persistence>
- <https://vercel.com/changelog/simplified-file-retrieval-from-vercel-sandbox-environments>
- <https://vercel.com/changelog/sandbox-sdk-file-permissions>

SSH execution uses argument-safe command construction and a native SSH session.
File operations use SFTP. No command may interpolate an unescaped path or env
value into a remote shell.

## REST surface

- `GET/POST /api/v1/orgs/{org}/assets`
- `GET/PATCH/DELETE /api/v1/orgs/{org}/assets/{id}`
- `GET /api/v1/orgs/{org}/assets/{id}/health`
- `GET/POST /api/v1/orgs/{org}/assets/{id}/links`
- `DELETE /api/v1/orgs/{org}/assets/{id}/links/{agent_id}`

All organization members may list and view assets. Only organization owners and
global administrators may create, update, delete, link, or unlink them. The
chart polls health only while asset nodes are visible. Create accepts `name`,
`description`, top-level `notes_for_agents`, and a server payload (`address`,
`port`, `user`, auth type, and optional expected host key). The response returns
the generated public key for SSH-key authentication and never returns passwords
or encrypted credentials.

## MCP tools

- `list_assets`, `get_asset`
- `server_run_command`
- `server_list_commands`, `server_get_command`, `server_kill_command`
- `server_list_files`, `server_read_file`, `server_write_file`
- `server_ssh_access`
- owner management: `list_org_assets`, `get_org_asset`,
  `create_server_asset`, `update_server_asset`, `delete_asset`,
  `list_asset_links`, `link_asset`, `unlink_asset`, `get_asset_health`

All server tools select an asset by ID or org-scoped name and require a live
link from the invoking agent. The initial slice limited management to REST. The
2026-08-02 follow-up in `design/2026-08-02-org-asset-mcp-management.md` adds the
owner-management MCP surface to Chief of Staff without widening the linked
operational tools.

## SSH proxy

The proxy is a separate SSH listener. Its CA and server host keys are stable
Ed25519 keys derived for distinct purposes from the Helix encryption key. The
`server_ssh_access` MCP tool mints a fresh Ed25519 key and a one-hour user
certificate containing signed `(org_id, agent_id, asset_id)` scope. The SSH
username is the asset name. A session is accepted only when the certificate is
valid and that agent still has a live link to the named asset; authorization is
checked again before dialing upstream. Session channels bridge shell, exec, PTY,
resize, signals, stdin/stdout/stderr, and exit status. Other channel types,
including `direct-tcpip`, are rejected.

The MCP response includes the exact setup command, after which a sandboxed agent
can use `ssh -i <identity> -p <port> <asset-name>@<proxy-host>`. There is no
unauthenticated, password-to-proxy, or source-IP-based mode.

## CLI

`helix org assets` exposes `list`, `get`, `create server`, `update`, `health`,
`link`, `unlink`, and `delete`. Passwords are accepted only with
`--password-stdin`; they cannot be placed in shell history as a flag. SSH-key
create prints the public key that must be installed on the target server.

## Chart UI

“New” is the chart toolbar's single create action, with Agent, Topic, Processor,
and Asset entries. “New asset” remains the final right-click menu option. Create
and edit use one shared side-drawer surface matching the processor create/edit
pattern. The drawer selects the asset type first (only Server today), then shows
its typed fields. Agent notes are the final data field. Server nodes show one
combined health light: green only when both network and authenticated SSH checks
pass, yellow otherwise. The top navigation has a final Assets tab with a
`SimpleTable` registry view, health, endpoint, linked-agent count, and the same
create/edit drawer.

Asset-to-agent links render as blue “available to” edges. Asset cards expose a
compact connection handle on all four sides; dragging any handle onto an agent
creates the same authorization link as the edit drawer. Asset and agent handles
share the same neutral color and size. The graph refreshes each node's measured
handle bounds after server data changes so a newly rendered handle is immediately
connectable.

Right-click creation records the click in React Flow coordinates and persists a
centered `org_chart_positions` row after the entity is created. Agents, topics,
processors, and assets all use this path. The position domain accepts all four
node kinds. Personal viewport records also include the current graph node IDs;
when the graph changes, the chart fits the new graph instead of restoring a
camera that can leave a newly added node off-screen.

## Delivery order

1. Domain validation, GORM/memory repositories, JSON serialization, and key
   encryption tests.
2. Asset application service, link-derived tools, REST CRUD/link/health, and
   OpenAPI client generation.
3. Chart asset node, create/detail/link UI, persisted layout, and health lights.
4. SSH/SFTP executor plus agent-scoped MCP tools.
5. Authenticated SSH proxy and on-demand agent certificate material.
6. Go builds/tests, frontend build, live inner-Helix browser verification, then
   user-approved live remote-host SSH testing.

## Verification

- Domain/store multi-tenant, secret-redaction, key round-trip, link cascade, and
  link authorization tests.
- SSH proxy integration against an ephemeral in-process SSH target: a linked
  agent certificate runs an exec request end-to-end, receives stdout and exit
  status, and the same certificate is rejected immediately after unlink.
- `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` and targeted org tests.
- `./stack update_openapi`, then `cd frontend && yarn build`.
- Inner Helix browser: right-click the chart, confirm New asset is last, toggle
  password/key auth, create a server, see its public key and yellow health, verify
  its click-centered position through the chart-position API, see it without
  using Fit View, list it in the Assets tab, remount the chart, and clean it up.
- After explicit user approval: connect to the supplied remote host and test the
  immediately following operation through both MCP and `ssh asset@proxy`.

### Live remote verification (2026-08-02)

`ubuntu-1` was linked to `chief-of-staff` in `unmanned-org`. The live agent used
the MCP tools to list/get the asset and its notes, run a foreground command,
run/list/get/kill a detached command, write/list/read/delete a UTF-8 file, and
verify cleanup. The detached command reached `killed` without its end marker.

The initial proxy test found that the listener was running inside the API
container but its host binding had not been applied to the existing container.
After applying the compose binding, ordinary SSH succeeded. The durable routing
fix derives the advertised proxy hostname from `SANDBOX_API_URL` before falling
back to `SERVER_URL`; the agent then received `api:2224` and successfully ran an
ordinary SSH command through that internal endpoint.

The same test exposed a startup race: external-agent execution queried Hydra
before waiting for the agent WebSocket. The lookup now occurs after the existing
readiness hook. A live stop, automatic start, MCP call, and following chat all
completed without the prior session-not-found error.

### CI E2E coverage

`integration-test/api/assets_e2e_test.go` runs in Drone's existing
`api-integration-test` step on every push. It starts an ephemeral SSH/SFTP
target and drives the real Helix API, Postgres-backed org store, MCP gateway,
asset runtime, and SSH proxy. The test covers owner/member/non-member RBAC,
create/update persistence, health and host-key pinning, link-derived MCP tools,
foreground and detached command list/get/kill, file write/read/list, proxy SSH,
unlink revocation, and delete followed by recreate. It has no dependency on the
live `ubuntu-1` host or a CI credential.

The frontend suite separately asserts that saved asset coordinates are used,
linked agents produce a visible arrowed edge, and each asset card renders one
combined network/SSH status indicator.
