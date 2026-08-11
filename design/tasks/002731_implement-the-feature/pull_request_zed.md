# Mirror ACP elicitations to Helix and accept answers

## Summary

`crates/external_websocket_sync/` had no concept of elicitations. Its entry mapping matched
only `UserMessage | AssistantMessage | ToolCall`, so `AgentThreadEntry::Elicitation` was
silently dropped — a question the agent asked never reached Helix, and the turn blocked on
`respond_tx` until something killed it.

Everything needed was already reachable from this crate: `AcpThread::elicitation()` carries
the full request (message, verbatim `requestedSchema`, and the `toolCallId` on its scope),
and `respond_to_elicitation()` resolves the pending `respond_tx`. No `agent_ui` changes.

Pairs with the Helix PR, which renders and answers the question.

## Changes

- New `SyncEvent` variants: `ElicitationRequested`, `ElicitationResolved`,
  `ElicitationResync`, `ElicitationResponseAck`. `requested_schema` is serialized verbatim —
  Helix renders every control from it, so flattening would drop options the agent offered.
- Emission from `AcpThreadEvent::ElicitationRequested` / `ElicitationResponded`, plus an
  `Elicitation` arm in `EntryUpdated` — `respond_to_elicitation` and `cancel_elicitation`
  emit only that, and it is what stops the Helix card being answerable after the question
  is resolved elsewhere.
- New `respond_elicitation` command, wired through a dedicated GPUI task like the cancel
  path (the sequential creation loop is blocked awaiting the very turn that is waiting for
  the answer, so routing through it would deadlock). Reports `accepted` / `noop` /
  `not_found` back rather than leaving Helix guessing.
- **Heartbeat resync.** Any thread holding a pending elicitation re-affirms it every 15 s,
  and a reconnect triggers a full re-announcement. This is what lets Helix distinguish "the
  agent died" from "the Helix API restarted" — a reconnect alone proves nothing, since the
  commonest cause is an API rebuild while this process and its `respond_tx` live on.
- `elicitation_status_str()` is the single place Zed's `Canceled` maps to the wire's
  `cancelled`.

## Testing status

**NOT compiled.** There is no Rust toolchain in the environment this was written in (no
`cargo`, no `~/.cargo`, no `target/`); the build path is `./stack build-zed` from the helix
repo, which did not complete under the machine's load. ACP type shapes were verified
against docs.rs rather than by compiling — which caught two wrong assumptions
(`tool_call_id` is on `ElicitationScope::Session`, not on `CreateElicitationRequest`; accept
content is `BTreeMap<String, ElicitationContentValue>`, not raw JSON) — but that is not a
substitute for a build.

E2E phase and live verification are also outstanding.

Release Notes:

- Added mirroring of ACP elicitations (agent questions) to Helix, and a command to answer them
