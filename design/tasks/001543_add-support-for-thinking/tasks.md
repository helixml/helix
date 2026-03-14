# Implementation Tasks

- [ ] Add a debug log in `handleMessageAdded` (`websocket_external_agent_sync.go`) to print the raw content/type of messages received from the Claude Code agent during a SpecTask run
- [ ] Run a SpecTask with a Claude Code agent and inspect logs to determine the exact format of thinking output (structured block vs literal XML tag vs other)
- [ ] If thinking arrives as structured blocks in the ACP protocol: add detection and wrapping in `<think>...</think>` in the external agent message handler, mirroring the approach in `inference_agent.go`
- [ ] If thinking arrives as literal `<thinking>` XML text: extend `processThinkingTags()` in `Markdown.tsx` to also match `<thinking>` tag variant (in addition to existing `<think>`)
- [ ] Remove the debug log added in step 1
- [ ] Manually verify: run a SpecTask, confirm thinking output appears as a collapsed "💡 Thoughts" widget with streaming timer
- [ ] Verify historical messages with thinking content also render correctly (non-streaming path)
