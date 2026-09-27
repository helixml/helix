# Org Bot instance isolation

Org Bot instances can execute requests from users who are not trusted Helix
operators. Their sandbox defaults therefore differ from ordinary interactive
sessions and from a Bot's main session.

## Creation-time grants

An instance receives no project secrets unless their names are supplied in the
creation request. Only development-scoped secrets can be selected. The names,
not values, are stored in session metadata so restart and recovery paths apply
the same allowlist. A granted secret that has since been deleted fails the
instance start instead of silently running without it.

The allowlist governs environment injection only. If a Bot's instance profile
grants the `get_secret` org tool, the instance can read the secrets bound to
the Bot through it. Do not grant that tool to instances that serve untrusted
users.

Subscription credentials are rejected because a subscription is its owner's
personal login, not an appropriate tenant boundary. Instances must use API-key
credentials. The check runs at creation, on every sandbox start, and when the
running instance fetches its agent config, so switching the Bot to a
subscription later does not expose it.

The shared writable agent binary cache is not mounted into any Org Bot
container. This prevents one Bot from replacing executable state consumed by
another tenant. An instance keeps pinned agent binaries on its own disk.

## Resource and privilege defaults

Each instance defaults to:

- a dedicated 10 GB ext4 filesystem mounted at `/home/retro`;
- a 4096-task cgroup limit. An idle desktop instance already runs about 450
  tasks on a 12-core host, and thread pools grow with the core count;
- bounded tmpfs mounts for `/tmp`, `/var/tmp`, `/etc/claude-code` and, for
  headless instances, `/run/user/1000`;
- no container engine;
- for headless instances, `no-new-privileges`, which prevents the image's
  passwordless sudo rule from escalating the `retro` user.

With `no-new-privileges`, the image entrypoint also revokes the `retro` user's
write access to everything on the container's root filesystem before it drops
privileges. The disk and the tmpfs mounts are then the only storage the agent
can write, so the disk size is a complete host-disk boundary.

The disk size can be explicitly set from 1 through 1000 GB at creation. The
filesystem contains the complete home directory, workspace, tool state, and
caches. It persists across stops. Stopping an instance unmounts it and releases
its loop device; deleting the instance removes it. The orphan reaper removes the
disk of any session that no longer exists, in case a delete could not reach the
sandbox host.

An instance created before disks existed keeps its data: its first start with a
disk copies the old workspace onto the new disk. The old workspace directory is
left in place until the instance is deleted.

## Sudo

`sudo: true` is an explicit trust opt-in for headless instances. It disables
`no-new-privileges` so passwordless sudo works. `ubuntu-desktop` instances
always allow sudo, because GNOME startup configures devices as root.

A root process can write to the container's writable layer outside the
quota-backed home. Instances with sudo must not be used for an untrusted
third-party tenant when the home quota is expected to be a complete host-disk
boundary.
