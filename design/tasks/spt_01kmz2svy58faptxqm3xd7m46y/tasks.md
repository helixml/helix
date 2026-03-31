# Implementation Tasks

- [ ] Remove debug `eprintln!` at lines 540–543 of `zed/crates/external_websocket_sync/src/thread_service.rs`
- [ ] Change `let _ = crate::send_websocket_event(SyncEvent::MessageCompleted { ... })` (line 544) to use `.log_err()`
- [ ] Run `cargo test -p external_websocket_sync` and confirm tests pass
- [ ] Verify fix manually: start a spectask, trigger tool calls, check `response_entries` in DB — all text entries should have complete content
