# Org Bot instance isolation

Org Bot instances can execute requests from users who are not trusted Helix
operators. Their sandbox defaults therefore differ from ordinary interactive
sessions and from a Bot's main session.

## Creation-time grants

An instance receives no project secrets unless their names are supplied in the
creation request. Only development-scoped secrets can be selected. The names,
not values, are stored in session metadata so restart and recovery paths apply
the same allowlist.

Subscription credentials are rejected because their host-managed credential
state is not an appropriate tenant boundary. Instances must use API-key
credentials.

The shared writable agent binary cache is not mounted into any Org Bot
container. This prevents one Bot from replacing executable state consumed by
another tenant.

## Resource and privilege defaults

Each instance defaults to:

- a dedicated 10 GB ext4 filesystem mounted at `/home/retro`;
- a 512-process cgroup limit;
- bounded tmpfs mounts for `/tmp` and `/var/tmp`;
- no container engine;
- `no-new-privileges`, which prevents the image's passwordless sudo rule from
  escalating the `retro` user.

The disk size can be explicitly set from 1 through 1000 GB at creation. The
filesystem contains the complete home directory, workspace, tool state, and
caches. It persists across stops and is unmounted and removed when the instance
is deleted.

`sudo: true` is an explicit trust opt-in. It disables `no-new-privileges` so
passwordless sudo works. A root process can write to the container's writable
layer outside the quota-backed home, so this mode must not be used for an
untrusted third-party tenant when the home quota is expected to be a complete
host-disk boundary.
