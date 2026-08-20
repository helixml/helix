# Project artifacts

## Goal

Let users and coding agents publish a self-contained HTML file or a compiled static SPA as a
project resource. Artifacts inherit project RBAC for management and private viewing, are stored in
Helix's filestore, and are served without starting a sandbox or application process.

The project UI gains:

- `/orgs/:org_id/projects/:id/artifacts` for inventory and management;
- an **Artifacts** action from the project specs page;
- a safe preview/open URL at `/artifacts/:artifact_id/`;
- optional publishing on a dedicated Helix subdomain.

The CLI gains `helix artifact create|update|list|get|delete`. `--project` defaults to
`HELIX_PROJECT_ID`, so both SpecTask containers and helix-org Worker containers operate on their
own project without rediscovering it.

## Claude Artifacts reference

Claude's current product establishes the useful interaction model, not a wire-compatible API:

- artifacts are substantial, self-contained documents or apps shown separately from chat;
- HTML, SVG, React components, documents, and diagrams are common artifact types;
- edits produce selectable versions;
- organization sharing additionally checks project access;
- publishing creates a standalone URL and can be revoked;
- Claude Code artifacts update a private live URL in place.

Sources inspected on 2026-08-20:

- https://support.claude.com/en/articles/9487310-what-are-artifacts-and-how-do-i-use-them
- https://support.claude.com/en/articles/9547008-publish-and-share-artifacts

Helix v1 deliberately implements static files only. AI calls, persistent application data, MCP
calls from browser JavaScript, remixing, and collaborative data stores are separate products and
must not be smuggled into the static hosting API.

## Security boundary

Agent-authored JavaScript is untrusted. Serving it as ordinary same-origin HTML on the Helix app
hostname would let it read the CSRF cookie and issue authenticated API mutations as the viewer.
`HttpOnly` on the session cookie does not prevent this.

Public `/artifacts/:id/*` responses therefore use a CSP sandbox without `allow-same-origin`.
Scripts can execute, but the document receives an opaque origin and cannot act as the Helix
application. Responses also set `nosniff`, a restrictive permissions policy, and
`frame-ancestors 'self'`.

An opaque-origin private SPA cannot send Helix cookies on its script and stylesheet requests. A
private canonical URL therefore serves only a trusted bootstrap form after project authorization.
It POSTs a one-minute signed grant to the artifact's internal vhost; the grant never appears in a
query string, redirect URL, referrer, or access-log path. The vhost exchanges it for a host-only,
HttpOnly artifact session cookie and redirects to the requested artifact route with its query
string intact. Every file request
validates the signed cookie and re-checks the user's current project access, so membership changes
take effect without waiting for cookie expiry. The isolated host receives no Helix session or CSRF
cookies and can load a complete SPA normally.

Published subdomains remain explicit. Public artifacts receive a directly shareable hostname only
when requested. Switching a published artifact back to project visibility removes public access
but provisions the isolated private-delivery route. Private delivery requires Helix's vhost base
domain to be configured; insecure query-string bearer tokens are not a fallback.

HTML is static content, not a server-side workload. Helix never runs `npm`, build scripts, server
functions, or arbitrary code during upload or delivery. Agents build locally and upload the
resulting directory.

## Data model

### `Artifact`

- `id` (`art_…`, primary key)
- `project_id` (indexed)
- `organization_id` (indexed, denormalized for audit/search)
- `name`, `description`
- `kind`: `single_file` or `spa`
- `entrypoint` (normally `index.html`)
- `visibility`: `project` (default) or `public`
- `active_version_id`
- `created_by`, `updated_by`
- timestamps and soft-delete timestamp

An artifact is stable identity and mutable metadata. Access is always authorized against its
project; there is no second artifact ACL that can drift from project grants.

### `ArtifactVersion`

- `id` (`artv_…`, primary key)
- `artifact_id` (indexed)
- monotonic `version`
- filestore `storage_prefix`
- immutable file manifest (`path`, size, media type, SHA-256)
- total bytes, file count, aggregate content hash
- optional `source_session_id` and `source_spec_task_id`
- `created_by`, `created_at`

Each upload writes a new unique filestore prefix before atomically inserting the version and
moving `active_version_id`. Readers never observe a partially overwritten deployment. Concurrent
updates lock the artifact row while assigning the next version number.

Files live under:

```text
<FILE_PREFIX_GLOBAL>/projects/<project_id>/artifacts/<artifact_id>/versions/<version_id>/<path>
```

The server, not the client, constructs this prefix.

## API

Authenticated control-plane endpoints:

```text
GET    /api/v1/projects/{project_id}/artifacts
POST   /api/v1/projects/{project_id}/artifacts
GET    /api/v1/artifacts/{artifact_id}
PUT    /api/v1/artifacts/{artifact_id}
DELETE /api/v1/artifacts/{artifact_id}
GET    /api/v1/artifacts/{artifact_id}/versions
```

Create and update accept multipart form data. The `artifact` part is either one static file or a
ZIP containing a compiled SPA. Other fields are `name`, `description`, `entrypoint`, `visibility`,
`with_subdomain`, `source_session_id`, and `source_spec_task_id`. An update without an `artifact`
part changes metadata only.

Validation happens before any write:

- normalized relative paths only; reject absolute paths, `..`, backslashes, symlinks, and
  duplicate normalized paths;
- entrypoint must exist and must be HTML;
- bounded compressed request, uncompressed total, individual file size, and file count;
- reject encrypted or unsupported ZIP entries;
- verify optional provenance belongs to the same project;
- compute media types and hashes server-side.

Static delivery entrypoint:

```text
GET /artifacts/{artifact_id}/
GET /artifacts/{artifact_id}/{path...}
```

Directory paths resolve `index.html`. For SPAs, a missing extensionless route falls back to the
declared entrypoint. `HEAD`, ETags, conditional requests, byte lengths, and `Cache-Control:
no-cache` are supported. Public canonical delivery stays CSP-sandboxed. Private canonical delivery
performs the signed POST bootstrap to the isolated vhost described above. The mutable URL therefore
updates immediately while still permitting conditional browser caches.

## RBAC

- list/view/versions/private static view: project `Get`/`List`;
- create: project `Create`;
- metadata/content update and publish/unpublish: project `Update`;
- delete: project `Delete`;
- public subdomain delivery: no user identity, only when `visibility=public`;
- private vhost delivery: signed artifact cookie plus a fresh project `Get` authorization on every
  request.

All handlers load the artifact, then authorize its canonical `project_id`. No request field can
substitute another organization or project after creation.

## CLI and agent workflow

```bash
helix artifact create ./index.html --project prj_... --name dashboard
helix artifact create ./dist --project prj_... --name app --visibility public --subdomain
helix artifact update art_... ./dist
helix artifact update art_... --name "New name" --visibility project --subdomain=false
helix artifact list --project prj_... --json
helix artifact get art_...
helix artifact delete art_...
```

The content path is a positional argument. The CLI uploads a regular file directly or builds a
ZIP when the path is a directory, preserves POSIX relative paths, refuses symlinks, and uploads
through the client package. `create` uses `HELIX_PROJECT_ID` when `--project` is omitted.

The desktop images ship the `helix` CLI. Artifact commands additionally accept the runtime-native
`HELIX_API_URL` and `USER_API_TOKEN` when `HELIX_URL`/`HELIX_API_KEY` are absent. This avoids
duplicating credentials in container environment variables. Session/task provenance is attached
automatically only when the target equals `HELIX_PROJECT_ID`; an org Worker can publish to another
authorized project with `--project` without incorrectly claiming that its current session belongs
to the target.

This is the correct surface for SpecTask and helix-org agents. Adding four file-transfer MCP
wrappers to the org-graph server would violate its small-surface rule and force large application
bundles through JSON tool arguments. Agents create files with their shell/build tools and invoke
one CLI upload command. The artifact skill documents that path and the current project default.

## UI

`ArtifactsPage` follows existing project list conventions:

- generated API client wrapped by React Query in `artifactService.ts`;
- page header and project breadcrumb;
- **New Artifact** action;
- `ViewModeToggle` with `SimpleTable` and `CardGrid` views;
- cards/table show name, kind, visibility, current version, file count, update time, and URL;
- one vertical-dot action menu for Open, Edit/publish version, and Delete;
- create/update dialog accepts one HTML file or one ZIP bundle and exposes entrypoint and
  publication options;
- Open renders in a new tab and relies on the response CSP on the canonical hostname.

The specs page gets an **Artifacts** labeled action and the router gets
`org_project-artifacts` at `/orgs/:org_id/projects/:id/artifacts`.

## Subdomains

Add `artifact_static` to `VHostTargetKind`. `target_id` is the artifact ID. Default hostnames use
the existing allocation and reservation logic. The vhost middleware dispatches the request to
the artifact static handler instead of Hydra. Project-private artifacts always keep an internal
vhost for isolated authenticated delivery; its URL is not advertised as a public subdomain.

No port is used for artifact routes. Custom-domain management can reuse the existing vhost
verification primitives after v1; the first implementation creates/revokes default subdomains
only. A public artifact without `with_subdomain` has no vhost route and stays on the sandboxed
canonical URL. This satisfies static subdomain serving without coupling artifacts to project
web-service sandboxes.

## Deletion and lifecycle

Deleting an artifact removes its vhost routes, deletes its filestore subtree, and soft-deletes
the metadata. Project and organization deletion invoke the same cleanup for all child artifacts
before deleting their ownership graph. If filestore deletion fails, metadata remains so the
operation can be retried; Helix does not knowingly leave a live row pointing at missing content.

## Verification

Automated verification covers ZIP parsing/traversal rejection, path resolution and SPA fallback,
canonical/subdomain CSP policies, CLI directory archiving and agent environment fallback, vhost
routing regressions, server/client/CLI compilation, TypeScript compilation, and a production
frontend build.

Live-stack verification exercises create → serve → update → serve changed content → delete
→ 404 → recreate; anonymous private denial; the signed private-SPA bootstrap and authenticated
multi-file asset seam; private/public route transitions; a public ZIP SPA's entrypoint, nested
asset and client route; host-header subdomain dispatch; browser execution under the canonical CSP
sandbox; UI routing/dialogs; and cleanup of metadata, blobs, and vhost rows.

Desktop-image verification builds the Ubuntu image containing `/usr/local/bin/helix`. Its
container smoke test uses the same `HELIX_API_URL`, `USER_API_TOKEN`, and `HELIX_PROJECT_ID` path
that SpecTask and Worker sessions receive.

For every update/delete test, exercise the immediately following normal operation. In particular:
update then fetch the changed entrypoint; delete then recreate and open a same-named artifact.
