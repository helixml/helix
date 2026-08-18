# 2026-08-18 — Sandbox security analysis: verification and fixes

This is the follow-up to the white/grey-box review in
`infra/docs/2026-08-16-sandbox-security-analysis.md` (task 000389 "take a look
at the findings"). Every finding was re-verified against `main` (2026-08-18,
`0d1ba53aa`) and the safe, code-scoped subset was fixed in this PR. The rest
is deferred with rationale below — several are deployment-level (outer
hydra host, registry, TLS) or product-level (DinD architecture) and cannot be
shipped without a full-stack test environment and a release decision.

## Fixed in this PR

### H6/H9 — filestore path traversal (cross-tenant data access)
- `api/pkg/controller/filestore.go` — new `joinUnderBase(base, rel)` joins a
  caller path onto the base prefix and rejects any `..` segment, NUL bytes;
  a leading `/` is tolerated (stripped) to preserve the runner-file download
  flow that trims the user prefix into a slash-relative path.
- Applied at every untrusted join point: `GetFilestoreUserPath`,
  `GetFilestoreAppPath`, `GetFilestoreAppKnowledgePath`,
  `ensureFilestoreAppPath`, `ensureFilestoreSpecTaskAttachmentsPath`.
- `ExtractAppID` now rejects `apps/..` / `apps/.` (previously
  `path=apps/../../users` would resolve the app prefix to the filestore root).
- `ensureFilestoreUserPath` validates before the `CreateFolder` side effects.
- Tests: `api/pkg/controller/filestore_path_test.go` (join/extract matrix) and
  `FilestoreSuite` handler tests (`TestFilestoreList_PathTraversalRejected`
  covers the exact live payload `path=../usr_01kc.../sessions`, plus
  get-style and legit-path regression guards).

### H4/H8 — `oh-hallo-insecure-token` public backdoor
- `api/pkg/server/auth_middleware.go` — `knownInsecureTokens` denylist
  (`oh-hallo-insecure-token`, `inner-runner-token`); any request presenting a
  listed value is rejected regardless of deployment config, so a deployment
  that still ships the old default loses the backdoor the moment the API is
  upgraded. The set never shrinks.
- `api/pkg/server/server.go` — startup logs a loud error when the configured
  `RUNNER_TOKEN` is a known-insecure value; the external-agent RevDial
  WebSocket (`access_token`) check also refuses denylisted tokens.
- The hard-coded default is removed from every code path:
  `docker-compose.yaml`, `docker-compose.dev.yaml` (9 sites),
  `api/pkg/cli/spectask/spectask.go` + `api/pkg/cli/project/inspect.go`
  (`getToken` now fails with a clear message), `scripts/test-zed-sse-mcp.sh`,
  `scripts/kind_helm_install.sh` (generates random when unset),
  `for-mac/scripts/provision-vm.sh` (generates random at provision),
  `integration-test/api/integration_test.go` (random per run).
- `stack` gains `ensure_runner_token`: seeds a `RUNNER_TOKEN=$(openssl
  rand -hex 32)` into `.env` once, so the dev flow keeps working with a
  unique per-checkout token after the default is gone.
- Docs updated: `local-development.md`, `CLAUDE.md`,
  `skills/helix-org-cli/SKILL.md`, `api/pkg/cli/spectask/README.md`.
- **Operator note:** any deployment still running the old default must set a
  new unique `RUNNER_TOKEN` (and re-issue anything that used it) — otherwise
  its hydra RevDial and runner flows 401 by design.

### H2 — `RUNNER_TOKEN=inner-runner-token` in the Helix-in-Helix sample
- `api/pkg/services/sample_project_code_service.go` — the generated startup
  script now writes `RUNNER_TOKEN=$(openssl rand -hex 32)` into the inner
  `.env` (random per deployment). The old literal is also on the API
  denylist.

### H7 — `/debug/pprof/` public
- `api/pkg/server/server.go` — removed the `net/http/pprof` import and the
  unauthenticated `/debug/pprof/` route registration. No other component
  scrapes it (verified across `helix` and `infra`).

### C3/M7 (+ C2/M2 headless scope) — headless sandbox capabilities
- `api/pkg/hydra/devcontainer.go` (`buildHostConfig`) — containers of
  `ContainerTypeHeadless` (every Sandboxes-API runtime: headless-ubuntu,
  node22, python, custom image) now get:
  - Docker's **default seccomp + apparmor** profiles instead of
    `unconfined/unconfined`;
  - **private IPC** instead of host IPC;
  - **no capability upgrades** — the old blanket
    `CapAdd=[SYS_ADMIN, SYS_NICE, SYS_PTRACE, NET_RAW, MKNOD, NET_ADMIN]`
    is gone (Docker's default cap set remains, which already includes
    NET_RAW);
  - `MKNOD` **dropped** (useless in a headless container, escape-relevant
    only).
- Non-headless (desktop) containers are deliberately unchanged in this PR —
  see C1/C2 below.
- Net effect: the M7-confirmed `CapEff 00000000a8ac35fb` headless container
  no longer has SYS_ADMIN/SYS_PTRACE/NET_ADMIN, and its syscall filter is
  back on. "One kernel exploit away" for a headless sandbox becomes a
  kernel exploit on top of a restored syscall filter + MAC.

### M10 — `/buildkit-cache` 0777 RW bind into every sandbox
- `api/pkg/hydra/manager.go` — the shared dir is created `0700` (was `0777`).
- `api/pkg/hydra/devcontainer.go` — the `/buildkit-cache` bind into dev
  containers is now **read-only**. Nothing writes there today (the real CAS
  is the `buildkit_state` volume); per-session scratch should be a local
  tmpfs if a workflow ever needs it. This removes the cross-tenant writable
  scratch and the CAS-poison path if buildkitd's content root were ever
  pointed at the dir.

### Chart hardening (same class as H4)
- `charts/helix-controlplane` — postgres password default
  `oh-hallo-insecure-password` removed; empty `values.postgresql.auth.password`
  now renders `randAlphaNum 32` (deterministic per release, so the postgres
  pod and the controlplane env agree). Set an explicit password or use
  `existingSecret` for anything persistent.

## Verified still open — deferred with rationale

| Finding | Status | Why deferred |
|---|---|---|
| C1 (desktop `--privileged`) | Open | Root cause of the trivial escape, but the fix is the DinD architecture migration (rootless or fuse-overlayfs inner dockerd) — needs the desktop image rebuilt plus a full desktop E2E (streaming, GPU, Kind) that this environment cannot run. |
| C2 (seccomp/apparmor unconfined) | Partial | Headless fixed now; desktop keeps unconfined until the C1 migration (inner dockerd + overlay2 is the stated dependency). |
| C4 (docker socket in desktop) | Open | By product design (docker-in-desktop). Becomes acceptable once C1–C3 close for desktops. |
| C5 (unauthenticated shared BuildKit) | Open | Requires token plumbing across `manager.go` (buildkitd `--tlstokens`), the sandbox env, and the desktop-image `17-start-dockerd.sh` buildx bootstrap, plus fleet redeploy; buildx `remote` bootstrap token handling needs a live verification. Highest-risk-of-breaking-builds item on the list — do it as a dedicated change with a green CI build pipeline. |
| H1 (any org member → root exec) | Partial | Container side hardened for headless (C3); the Sandboxes API exec surface itself (non-root exec, `Cwd`/`Args` validation) and the still-privileged desktop runtime remain. |
| H3 (open registration / everyone-admin default) | Open (operational) | Code no longer auto-promotes first users (that was replaced by `ADMIN_USER_IDS`); remaining risk is deployments relying on the compose defaults (`AUTH_REGISTRATION_ENABLED` default true, `ADMIN_USER_IDS=all`). Flipping those defaults is a product decision affecting every self-hosted customer's signup flow. |
| H5/M9 (sandbox → host/Tailscale/Postgres network) | Open | Egress policy (separate sandbox network, no `172.19.0.0/16` / `100.64.0.0/10` route, proxy-only API egress) is deployment/network-level; needs the outer stack's docker network + iptables changed and verified. |
| H10 (anonymous push + mutable golden tags) | Open | Registry auth + immutable tags + repo allowlist in the docker-wrapper: registry-side config (outer deployment) plus wrapper change with a full build-pipeline verification. |
| M1 (custom image gate) | Safe as-is | Default `false` as the review recommends; keep it off unless an operator explicitly opts in. |
| M3 (plaintext HTTP control plane) | Open | Deployment TLS (the `docker-compose.tls.yaml` overlay exists); not code-scoped. |
| M4 (GPU device leak in nested DinD) | Open | Needs device cgroup enforcement at the outer (hydra host) level; GPU-fleet dependent to verify. |
| M5 (`NODE_TLS_REJECT_UNAUTHORIZED=0`) | Open | Set in the desktop image `ENV` (`Dockerfile.ubuntu-helix`, `Dockerfile.sway-helix`); removal needs a desktop image rebuild + verification that nothing in the desktop relies on it (Zed/proxy TLS). |
| M6 (BuildKit gRPC reachable from sandboxes) | Open | Same root as C5. |
| M8 (desktop-bridge `/exec` unauthenticated; `cat`/`claude` allowlist) | Open | Correct fix is token-auth on the API→desktop hop (the bridge already receives `USER_API_TOKEN`) plus tightening the allowlist; needs a live desktop (RevDial E2E) to verify, and `cat` is part of a legitimate debug path so it shouldn't be dropped blindly. |
| L1 (embedded NATS) | Note | Not externally exposed; pprof (its leak amplifier) is gone. |
| L2/L3 | See C5/H10 / M5 | |

## Recommended next steps (order)

1. **C5/H10 as one build-pipeline security project:** BuildKit TLS-token
   auth (admin-only `Prune`, no worker `network=host`), registry auth +
   immutable/digest-pinned `buildcache/*` tags, wrapper repo allowlist.
2. **C1 migration:** rootless or fuse-overlayfs inner dockerd; re-enable
   default seccomp/apparmor + host-IPC removal for desktops (rest of C2/M2)
   in the same program, with desktop E2E (streaming, GPU, nested Kind).
3. **H5/M9 network segmentation** for sandboxes (proxy-only egress).
4. **H3 defaults:** decide product stance on `registration_enabled` /
   `admin_user_ids` compose defaults; document per-deployment requirements.
5. **M8:** authenticate the desktop-bridge `/exec` hop; then prune the
   allowlist.
6. Ops: rotate `RUNNER_TOKEN` on every deployment that had the old default
   (forced by the denylist now), review `/api/v1/config` exposure (LOW), and
   drop the leftover registry probe tag
   `buildcache/keeltest:secreview-probe-1786911739` from the registry host.

## Test evidence

- `go build ./...` — OK (api module).
- `CGO_ENABLED=0 go test ./pkg/controller/ ./pkg/sandbox/ ./pkg/services/ ./pkg/cli/...` — OK.
- `CGO_ENABLED=0 go test ./pkg/server/` — OK except two pre-existing failures
  (`TestInProcClient_DeleteLinkedAgent*`), reproduced identical on a clean
  `main` worktree baseline (unrelated to this change).
- `CGO_ENABLED=0 go test ./pkg/hydra/` — OK except pre-existing
  `TestDiskPressureSuite` (environment-dependent; identical failure on clean
  baseline).
- `bash -n` on `stack`, `scripts/test-zed-sse-mcp.sh`,
  `scripts/kind_helm_install.sh`, `for-mac/scripts/provision-vm.sh` — OK.
- `integration-test/api` compiles.
- NOT tested: live sandbox/desktop runtime behaviour of the new headless
  security profile and the RO `/buildkit-cache` mount (no hydra host /
  desktop image build available in this review environment).
