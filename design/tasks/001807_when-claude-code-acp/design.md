# Design: Auto-Approve ACP Permission Requests for Helix

## Architecture

The change is a single-point intervention in the ACP `request_permission` handler, behind the existing `#[cfg(feature = "external_websocket_sync")]` feature gate.

## Key File

**`crates/agent_servers/src/acp.rs`** — `ClientDelegate::request_permission()` (line ~1448)

This method is the entry point for ALL ACP permission requests. Currently it:
1. Looks up the session/thread
2. Calls `thread.request_tool_call_authorization()` which creates a `WaitingForConfirmation` status with a oneshot channel
3. Emits `ToolAuthorizationRequested` event (UI renders buttons)
4. Awaits the oneshot channel for user response

## Approach

Add a `#[cfg(feature = "external_websocket_sync")]` block at the top of `request_permission()` that:

1. Finds the first `AllowOnce` option from `arguments.options` (falling back to `AllowAlways`, then first option)
2. Returns `acp::RequestPermissionResponse::new(acp::RequestPermissionOutcome::Selected(acp::SelectedPermissionOutcome::new(option.option_id.clone())))` immediately
3. Skips the thread authorization / UI flow entirely

```rust
async fn request_permission(
    &self,
    arguments: acp::RequestPermissionRequest,
) -> Result<acp::RequestPermissionResponse, acp::Error> {
    #[cfg(feature = "external_websocket_sync")]
    {
        // Auto-approve: find the best allow option
        let option = arguments.options.iter()
            .find(|o| o.kind == acp::PermissionOptionKind::AllowOnce)
            .or_else(|| arguments.options.iter().find(|o| o.kind == acp::PermissionOptionKind::AllowAlways))
            .or_else(|| arguments.options.first());

        if let Some(option) = option {
            return Ok(acp::RequestPermissionResponse::new(
                acp::RequestPermissionOutcome::Selected(
                    acp::SelectedPermissionOutcome::new(option.option_id.clone())
                )
            ));
        }
    }

    // ... existing flow unchanged ...
}
```

## Why This Approach

- **Minimal blast radius**: One `#[cfg]` block in one method. No new files, no new settings, no protocol changes.
- **Correct layer**: Intercepts at the ACP server → client boundary, before any UI state is created.
- **No orphan state**: Since we return before calling `request_tool_call_authorization`, no `WaitingForConfirmation` entry is created that would need cleanup.
- **Feature-gated**: Only affects Helix builds. Normal Zed is untouched.

## Codebase Patterns

- This repo uses `#[cfg(feature = "external_websocket_sync")]` for all Helix-specific changes (per `CLAUDE.md` porting guide)
- The `agent_client_protocol` crate (v0.10.2) provides the ACP types (`RequestPermissionRequest`, `PermissionOption`, `PermissionOptionKind`, etc.)
- `acp::PermissionOptionKind` has variants: `AllowOnce`, `AllowAlways`, `RejectOnce`, `RejectAlways`
- `acp::PermissionOption` has fields: `option_id`, `kind`, `name`

## Alternatives Considered

1. **Auto-approve in `conversation_view.rs` on `ToolAuthorizationRequested` event**: Would work but creates and immediately resolves a `WaitingForConfirmation` state — unnecessary churn. The ACP server level is cleaner.
2. **Add a new setting**: Over-engineered for this use case. The `external_websocket_sync` feature already means "Helix autonomous mode."
3. **Only auto-approve `ExitPlanMode`**: Would need to inspect tool name from meta. But any permission prompt blocks autonomous execution, so all should be auto-approved.
