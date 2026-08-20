# Project artifacts

## Goal

Let users and coding agents publish a self-contained HTML file or a compiled static SPA as a
project resource. Artifacts inherit project RBAC for management and private viewing, are stored in
Helix's filestore, and are served without starting a sandbox or application process.

The project UI gains:

- `/orgs/:org_id/projects/:id/artifacts` for inventory and management;
- an **Artifacts** action from the project specs page;
- a permission-checked viewer at `/artifacts/:artifact_id` with a share control;
- automatic isolated subdomain publishing when visibility is public.

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

Agent-authored JavaScript is untrusted. `/artifacts/:id` therefore renders only the trusted Helix
viewer toolbar. Artifact bytes load in a cross-origin iframe with a browser sandbox that permits
scripts and same-origin access only inside the isolated artifact hostname. The artifact origin
receives no Helix session or CSRF cookies. Its CSP restricts executable content to the artifact
origin, disables objects and sensitive browser capabilities, suppresses referrers, and permits
framing only by the configured Helix viewer origin.

Private iframe navigation first enters `/artifacts/:id/embed`, which checks project access and
POSTs a one-minute signed grant to the artifact's internal vhost. The grant never appears in a
query string, redirect URL, referrer, or access-log path. The vhost exchanges it for a host-only,
HttpOnly artifact session cookie. Every private file request validates the cookie and re-checks
current project access. The private hostname is never returned as `subdomain_url` or linked by the
UI. Public visibility exposes the same isolated hostname without user identity; returning to
project visibility immediately removes anonymous access while retaining the internal route for
the viewer.

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
`source_session_id`, and `source_spec_task_id`. `with_subdomain` remains a deprecated compatibility
field; public visibility now always allocates a share hostname. An update without an `artifact`
part changes metadata only.

Validation happens before any write:

- normalized relative paths only; reject absolute paths, `..`, backslashes, symlinks, and
  duplicate normalized paths;
- entrypoint must exist and must be HTML;
- bounded compressed request, uncompressed total, individual file size, and file count;
- reject encrypted or unsupported ZIP entries;
- verify optional provenance belongs to the same project;
- compute media types and hashes server-side.

Viewer and isolated-origin entrypoints:

```text
GET /artifacts/{artifact_id}                 # Helix viewer
GET /artifacts/{artifact_id}/embed           # iframe origin handoff
GET https://{artifact-subdomain}/{path...}   # static bytes
```

Directory paths resolve `index.html`. For SPAs, a missing extensionless route falls back to the
declared entrypoint. `HEAD`, ETags, conditional requests, byte lengths, and `Cache-Control:
no-cache` are supported. The stable viewer URL updates immediately while immutable content
versions remain cache-addressable through ETags.

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
helix artifact create ./dist --project prj_... --name app --visibility public
helix artifact update art_... ./dist
helix artifact update art_... --name "New name" --visibility project
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
  visibility;
- Open renders `/artifacts/:id`, whose top bar shows visibility and a Share menu;
- the viewer iframe loads only the isolated artifact origin with `sandbox` and `no-referrer`.

The specs page gets an **Artifacts** labeled action and the router gets
`org_project-artifacts` at `/orgs/:org_id/projects/:id/artifacts`.

## Subdomains

Add `artifact_static` to `VHostTargetKind`. `target_id` is the artifact ID. Every artifact receives
one default isolated hostname through the existing allocation and reservation logic. The vhost
middleware dispatches it to the static handler instead of Hydra. Project-private artifacts use the
hostname only through the authenticated viewer bootstrap and do not advertise it. Public artifacts
return it as `subdomain_url`. No port or sandbox runtime is involved.

## Deletion and lifecycle

Deleting an artifact removes its vhost routes, deletes its filestore subtree, and soft-deletes
the metadata. Project and organization deletion invoke the same cleanup for all child artifacts
before deleting their ownership graph. If filestore deletion fails, metadata remains so the
operation can be retried; Helix does not knowingly leave a live row pointing at missing content.

## Verification

Automated verification covers ZIP parsing/traversal rejection, path resolution and SPA fallback,
viewer/subdomain CSP policies, CLI directory archiving and agent environment fallback, vhost
routing regressions, server/client/CLI compilation, TypeScript compilation, and a production
frontend build.

Live-stack verification exercises create → serve → update → serve changed content → delete
→ 404 → recreate; anonymous private denial; the signed private-SPA bootstrap and authenticated
multi-file asset seam; private/public route transitions; a public ZIP SPA's entrypoint, nested
asset and client route; host-header subdomain dispatch; browser execution in the isolated iframe;
viewer Share transitions; UI routing/dialogs; and cleanup of metadata, blobs, and vhost rows.

Desktop-image verification builds the Ubuntu image containing `/usr/local/bin/helix`. Its
container smoke test uses the same `HELIX_API_URL`, `USER_API_TOKEN`, and `HELIX_PROJECT_ID` path
that SpecTask and Worker sessions receive.

For every update/delete test, exercise the immediately following normal operation. In particular:
update then fetch the changed entrypoint; delete then recreate and open a same-named artifact.
