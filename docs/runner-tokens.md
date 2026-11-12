# Per-runner tokens

Sandbox hosts authenticate to the control plane with `RUNNER_TOKEN`. By default
every host shares one value, so the control plane cannot tell hosts apart: any
holder of the shared token can register another host's `hydra-<id>` or
`sandbox-<id>` tunnel and receive its traffic until that host reconnects.

A **per-runner token** is bound to one runner id, the host's
`SANDBOX_INSTANCE_ID`. With it, a host can only:

- register its own RevDial tunnels (`hydra-<id>`, `sandbox-<id>`);
- open the runner websockets as `<id>`;
- register, heartbeat, deregister and read disk history for sandbox `<id>`;
- read runner `<id>`'s profile assignment, and only the profile assigned to it;
- list sandboxes: it sees only its own host's row.
- read agent credentials and sandbox configuration only for sessions running
  on sandbox `<id>`.

Using it for any other runner id gets `403`. A revoked or unknown token gets
`401`. Everything else the shared token can do, a per-runner token can do too,
so hosts need no other change.

The control plane stores only a SHA-256 hash of each token. The token itself is
shown once, when it is created.

## Rotating hosts to per-runner tokens

1. **Find each host's runner id.** It is the host's `SANDBOX_INSTANCE_ID`, the
   `<id>` in its `hydra-<id>` tunnel. Mint tokens for the bare id: `gpu-host-1`,
   not `hydra-gpu-host-1`. A `hydra-` or `sandbox-` prefixed id is refused,
   since the token already covers both of the host's tunnel ids.
   Hosts that use the shared token log a one-time warning per id on the
   control plane, naming the id and the command to run:

   ```
   Runner is using the shared RUNNER_TOKEN, which is deprecated for runner hosts ...
   ```

2. **Mint a token per host** (as a global admin, with `HELIX_URL` and
   `HELIX_API_KEY` set):

   ```bash
   helix runner token create --runner-id gpu-host-1 --name "gpu-host-1 (rack 3)"
   ```

   The API equivalent is `POST /api/v1/admin/runner-tokens` with
   `{"runner_id": "...", "name": "..."}`.

3. **Swap the value on the host.** The variable keeps its name; the server
   recognizes the token type.
   - **Docker Compose / `install.sh` hosts:** set `RUNNER_TOKEN=<new token>` in
     the host's `.env` and make sure `SANDBOX_INSTANCE_ID` is set to the same id
     the token was minted for, then `docker compose up -d`.
   - **Helm (`helix-sandbox` chart):** a token is per host, so install one
     release per host with a fixed id and the token in its own Secret:

     ```yaml
     sandbox:
       instanceId: gpu-host-1            # must match --runner-id
       runnerTokenExistingSecret: helix-runner-token-gpu-host-1
       runnerTokenExistingSecretKey: token
     ```

     Without `sandbox.instanceId` the chart uses the pod name as the id, and
     all replicas of a release read the same Secret, which cannot work with
     per-host tokens.

4. **Check** with `helix runner token list` and the control plane logs: the
   host's tunnels register and the deprecation warning stops for that id.

5. **Retire the shared token for hosts.** Once every host has its own token,
   set on the control plane:

   ```bash
   RUNNER_REQUIRE_PER_HOST_TOKENS=true
   ```

   (Helm `helix-controlplane`: add it under `controlplane.extraEnv`.) The shared
   `RUNNER_TOKEN` then gets `403` on every runner-identity route above. It
   still authenticates the control plane's own internal calls, so keep it set.

   Do not turn this on while the control plane provisions hosts itself (the
   managed compute pool): those hosts are started with the shared token.

## Revoking

```bash
helix runner token list --runner-id gpu-host-1
helix runner token revoke rtok_...
```

New requests with a revoked token are refused immediately, and the RevDial
tunnels it registered are closed at once: the control plane stops dispatching
to that host until it registers again with a valid token. A tunnel the host
has since re-registered with another credential is left alone. Websocket
sessions already open with the token end when they next reconnect.

The token row keeps who created and who revoked it, and when; every mint and
revoke request is also in the request audit log.

## NATS proxy

`RUNNER_NATS_PROXY_ENABLED` exposes the embedded NATS bus at
`/api/v1/runner/{id}/ws`. NATS authenticates with the shared token only and has
no per-runner subject permissions, so per-runner tokens do not work there.
Leave it off (the default). With `RUNNER_REQUIRE_PER_HOST_TOKENS=true` the
route is not registered even if the proxy is enabled.
