# Rootless web-service data directory ownership

Date: 2026-10-16
Status: implemented; production rollout pending review

## Finding

Rootless web services bind-mount project data at `/data`. The sandbox
controller already makes this path persistent and project-scoped in
`api/pkg/sandbox/controller_provision.go`. Hydra previously created a fresh
mount source with `os.MkdirAll(source, 0755)`, which left it owned by root.
The rootless Docker engine runs as the generic instance user (UID 1000), so a
database such as PostgreSQL could not create its bind-mounted child directory.
The manual test passed only after that directory was chowned.

## Fix

`prepareMountSources` in `api/pkg/hydra/devcontainer.go` now gives special
treatment only to a writable, persistent `/data` bind used by a rootless
engine. It validates a canonical absolute non-root path, opens each component
with `O_NOFOLLOW`, and fails before container creation for invalid paths,
symlinks, or non-directories. A fresh directory, or an existing empty
root-owned directory with the default `0755` mode, is changed to the instance
owner (UID/GID 1000). A fresh leaf still gets the process umask; existing
populated directories, custom owners, and custom modes are left unchanged.
There is no recursive chown. Rootful, non-persistent, read-only, ordinary bind,
and named-volume mounts retain their previous behavior. The existing rollout
flags are unchanged. Database images still choose their own users inside the
rootless namespace.

## Evidence

Focused Hydra tests and the desktop/web-service provisioning tests pass:

```text
go test ./api/pkg/hydra -run 'TestPrepareMountSources' -count=1
go test ./api/pkg/sandbox -run '^TestProvisionSuite$' -testify.m '^(TestProvisionDesktopRootlessSetsUnprivilegedRequest|TestProvisionRootlessWebServiceSetsPersistentDataRequest)$' -count=1
```

On Prime Linux, the ownership unit test passed. A live rootless Docker test
created the `1000:1000` data directory, started `postgres:17-alpine` without
manual chown, preserved SQL across restart, retained the PostgreSQL mapped
owner `100069`, and cleaned up its test resources.

The full Hydra suite was attempted on macOS but reached platform-dependent
tests requiring Linux cgroup limits, GNU `cp --reflink`, and system mount
points. The full Hydra binary and runner were not deployed; Meta was unchanged.
Persistence across replacement of the outer sandbox and production behavior
remain to be verified after review and rollout.
Populated root-owned storage needs a separate migration decision; this fix
does not convert it automatically.
