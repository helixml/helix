# Fix three restart-surfaced WebSocket sync bugs (#2641, #2642, #2643)

## Summary
Three production bugs surfaced by an `api` container restart on a long-running org
deployment, all rooted in the Helix↔Zed WebSocket sync correlation surface. Forensic
map and fix design: `helix-specs` `design/2026-06-19-request-id-routing-forensic-map.md`
and `design/2026-06-19-fix-restart-surfaced-websocket-bugs.md`.

Each fix is an independent commit so they can be reviewed (or reverted) separately.

## Changes

**#2642 — chat-path prompts silently dropped + notify storm** (`fix(api): stop dropping…`)
- `NotifyExternalAgentOfNewInteraction` no longer sends `chat_message` with `role:"user"`.
  The Zed client drops `role=="user"` as a UI-sync echo (`websocket_sync.rs:421`), so
  genuine chat-path prompts were discarded → interaction stuck `waiting` → "empty response".
  The queue path never set `role` and works; this matches it. (Zed's `role` is optional and
  unused after the echo check — no Zed change needed.)
- `startChatSessionHandler` now notifies only the newly-appended interaction instead of
  re-deriving a start index from a generation-boundary scan that failed whenever all
  interactions shared the current generation — which re-notified the whole history
  (~1381 `Notify` calls for one message → "send channel full").

**#2643 — reused-thread response dropped after restart** (`fix(api): revert risky #2643…`)
- **This PR does NOT fix #2643 in code — it only improves diagnosability.** A re-review
  found the true cause is a *divergence between two resolvers*: `handleMessageCompleted`
  resolves the interaction via the `request_id`→interaction mapping, while
  `getOrCreateStreamingContext` resolves via most-recent-`waiting`/restart-recovery. After a
  restart these can pick different interactions, so streamed content lands on one (or is
  dropped) while the completion marks another empty.
- The only #2643 change kept here is **loud logging** on the unroutable `message_added` drop
  (now includes `acp_thread_id`+`request_id`). An earlier attempt to broaden the restart
  recovery scan was **reverted** — it was an unverified guess that could misroute by
  resurrecting a stale interrupted turn behind a more-recent cancelled/errored one.
- The real fix — an explicit per-session current-turn pointer used by *both* resolvers — is
  written up in `helix-specs .../architecture-simplifications.md` §1 and needs live
  verification; recommended as the follow-on rather than shipped unverified.

**#2641 — stale `api` IP pinned in desktop `/etc/hosts`** (`fix(hydra): resolve api via…`)
- Desktops baked `api`'s IP into `/etc/hosts` at creation (`buildExtraHosts`); the immutable
  pin went stale on any API restart and (since `/etc/hosts` beats DNS) shadowed the dynamic
  path. On the default-bridge path, point the desktop resolver at the sandbox dns-proxy
  (`HostConfig.DNS = sandboxDNSGateway()`) and drop the pin, so `api` re-resolves to its
  current IP every reconnect. Taught the dns-proxy an `outer-api`→`api` alias (default on)
  for the Helix-in-Helix shadow-proof name. Non-bridge networking keeps the legacy path.

## Testing
- `go build ./pkg/hydra/ ./pkg/server/` and the `dns-proxy` module build clean.
- Zed-side behaviour for #2642 confirmed by reading `crates/external_websocket_sync`.
- **NOT live-verified against a connected Zed.** The inner dev environment could not
  provision a live desktop (the startup `build-sandbox`/`build-ubuntu` step failed on an
  unrelated qwen-code `npm run bundle` error → no `helix-ubuntu` image in the inner dockerd
  → Zed never connects). The mandated live tests — chat-path turn, restart-on-reused-thread,
  dedup, concurrent turns, full `stop`/`start` for #2641, H-in-H `outer-api`, and the
  `TestWebSocketSyncSuite` + `run_docker_e2e.sh` suites — must be run on a working
  environment / in CI before merge. #2641 in particular is high-blast-radius networking.
