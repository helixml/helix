# Implementation Tasks

## Settings Layer

- [ ] Add `auto_follow_agent: Option<bool>` field to `AgentSettingsContent` in `crates/settings_content/src/agent.rs`
  - Include doc comment: "Whether to automatically follow the agent's activity when sending messages. Default: true"
- [ ] Add `auto_follow_agent: bool` field to `AgentSettings` struct in `crates/agent_settings/src/agent_settings.rs`
- [ ] Update `from_settings` impl to map the new field with `.unwrap_or(true)` default

## Thread View Layer

- [ ] Import `AgentSettings` in `crates/agent_ui/src/acp/thread_view/active_thread.rs` (if not already imported)
- [ ] Update `AcpThreadView::new` to initialize `should_be_following` from `AgentSettings::get_global(cx).auto_follow_agent`

## Testing

- [ ] Manual test: New thread auto-follows agent activity by default
- [ ] Manual test: Setting `"agent": { "auto_follow_agent": false }` disables auto-follow
- [ ] Manual test: Toggle button still works during generation
- [ ] Manual test: Turning off follow mid-generation stays off until next message