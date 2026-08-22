# Project artifacts

## Goal

Let users and coding agents publish a self-contained HTML file, a compiled static SPA, a PDF, or
an image as a project resource. Artifacts inherit project RBAC for management and private viewing,
are stored in Helix's filestore, and are served without starting a sandbox or application process.

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
viewer toolbar. HTML and SPA bytes load in a cross-origin iframe with a browser sandbox that permits
scripts and same-origin access only inside the isolated artifact hostname. The artifact origin
receives no Helix session or CSRF cookies. Its CSP restricts executable content to the artifact
origin, disables objects and sensitive browser capabilities, suppresses referrers, and permits
framing only by the configured Helix viewer origin.

Private iframe navigation first enters `/artifacts/:id/embed`, which checks project access and
redirects the iframe to a random, expiring vhost route bound to that user and artifact. Every file
request validates the route expiry and re-checks current project access. This avoids third-party
cookies, which Safari blocks when local Helix runs on an IP address and artifact content runs on a
wildcard hostname. The private hostname is suppressed from API responses and UI links, and the
artifact response's `no-referrer` policy prevents it from leaking through subresource requests.
Public visibility uses the stable isolated hostname without user identity; returning to project
visibility immediately removes anonymous access from that stable route.

Chrome's native PDF viewer cannot run inside that cross-origin sandbox. PDFs therefore use
`/artifacts/:id/document`, a same-origin, permission-checked route restricted to `kind=pdf` and
served as `application/pdf` with `nosniff`, inline disposition, and a `default-src 'none'` CSP.
The browser extension renderer remains isolated while the viewer retains its Helix toolbar.

The trusted `/artifacts/:id` viewer is also reachable without a Helix session when the artifact is
public. Its unauthenticated API returns only viewer display metadata and `can_edit=false`; the
toolbar offers Login instead of Share. Private viewer metadata still requires project `Get`, and
the sharing control is rendered only when the response confirms project `Update` access.

HTML is static content, not a server-side workload. Helix never runs `npm`, build scripts, server
functions, or arbitrary code during upload or delivery. Agents build locally and upload the
resulting directory.

## Data model

### `Artifact`

- `id` (`art_…`, primary key)
- `project_id` (indexed)
- `organization_id` (indexed, denormalized for audit/search)
- `name`, `description`
- `kind`: `single_file`, `spa`, `pdf`, or `image`
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

Create and update accept multipart form data. The `artifact` part is HTML, PDF, an image, or a ZIP
containing a compiled SPA. Other fields are `name`, `description`, `entrypoint`, `visibility`,
`source_session_id`, and `source_spec_task_id`. `with_subdomain` remains a deprecated compatibility
field; public visibility now always allocates a share hostname. An update without an `artifact`
part changes metadata only.

Validation happens before any write:

- normalized relative paths only; reject absolute paths, `..`, backslashes, symlinks, and
  duplicate normalized paths;
- entrypoint must exist; bundles require HTML, while a single PDF or image is its own entrypoint;
- bounded compressed request, uncompressed total, individual file size, and file count;
- reject encrypted or unsupported ZIP entries;
- verify optional provenance belongs to the same project;
- compute media types and hashes server-side.

Viewer and isolated-origin entrypoints:

```text
GET /artifacts/{artifact_id}                 # Helix viewer
GET /artifacts/{artifact_id}/embed           # iframe origin handoff
GET /artifacts/{artifact_id}/document        # permission-checked native PDF delivery
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
- public viewer metadata: no user identity, reduced response shape, never editable;
- private vhost delivery: unexpired user-bound route plus a fresh project `Get` authorization on
  every request.

All handlers load the artifact, then authorize its canonical `project_id`. No request field can
substitute another organization or project after creation.

## CLI and agent workflow

```bash
helix artifact create ./index.html --project prj_... --name dashboard
helix artifact create ./dist --project prj_... --name app --visibility public
helix artifact create ./report.pdf --project prj_... --name report
helix artifact create ./diagram.png --project prj_... --name diagram
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
- one compact vertical-dot action menu with icons for Open, Edit/publish, and Delete;
- create/update dialog accepts HTML, a ZIP bundle, PDF, or an image and exposes entrypoint only
  where HTML routing needs one;
- Open renders `/artifacts/:id`, whose top bar shows visibility and a Share menu;
- HTML viewers load only the isolated artifact origin with `sandbox` and `no-referrer`; PDFs use
  the permission-checked document route and browser-native renderer, while images use a contained
  responsive image view.

The specs page gets an **Artifacts** labeled action and the router gets
`org_project-artifacts` at `/orgs/:org_id/projects/:id/artifacts`.

## Subdomains

Add `artifact_static` to `VHostTargetKind`. `target_id` is the artifact ID. Every artifact receives
one default isolated hostname through the existing allocation and reservation logic. The vhost
middleware dispatches it to the static handler instead of Hydra. Private viewer loads receive an
additional random, user-bound route with an eight-hour expiry; these routes are reused per user,
removed when expired, and never advertised. Public artifacts return only the stable hostname as
`subdomain_url`. URLs use the same external protocol and port configuration as spec-task previews
(`PREVIEW_URL_HTTPS` plus the port from `SERVER_URL`); no sandbox runtime or artifact application
port is involved.

## Deletion and lifecycle

Deleting an artifact removes its vhost routes, deletes its filestore subtree, and soft-deletes
the metadata. Project and organization deletion invoke the same cleanup for all child artifacts
before deleting their ownership graph. If filestore deletion fails, metadata remains so the
operation can be retried; Helix does not knowingly leave a live row pointing at missing content.

## Verification

Automated verification covers ZIP parsing/traversal rejection, PDF/image kind and entrypoint
validation, path resolution and SPA fallback,
viewer/subdomain CSP policies, CLI directory archiving and agent environment fallback, vhost
routing regressions, server/client/CLI compilation, TypeScript compilation, and a production
frontend build.

Live-stack verification exercises create → serve → update → serve changed content → delete
→ 404 → recreate; anonymous private denial; the signed private-SPA bootstrap and authenticated
multi-file asset seam; private/public route transitions; a public ZIP SPA's entrypoint, nested
asset and client route; public PDF and image MIME delivery; Chrome's native PDF embedder and the
responsive image viewer; host-header subdomain dispatch; browser execution in the isolated iframe;
viewer Share transitions; UI routing/dialogs; and cleanup of metadata, blobs, and vhost rows.

Desktop-image verification builds the Ubuntu image containing `/usr/local/bin/helix`. Its
container smoke test uses the same `HELIX_API_URL`, `USER_API_TOKEN`, and `HELIX_PROJECT_ID` path
that SpecTask and Worker sessions receive.

For every update/delete test, exercise the immediately following normal operation. In particular:
update then fetch the changed entrypoint; delete then recreate and open a same-named artifact.
