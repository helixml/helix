# Add web search instructions to agent prompts

## Summary
Add a "Web Search" section to both the planning and implementation prompt templates, telling agents they can use the `chrome-devtools` MCP server with DuckDuckGo to search the web.

## Changes
- Added "Web Search" section to planning prompt in `spec_task_prompts.go`
- Added "Web Search" section to implementation prompt in `agent_instruction_service.go`
