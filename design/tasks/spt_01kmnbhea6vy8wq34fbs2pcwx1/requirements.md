# Requirements

## User Story

As a user reviewing agent implementation work, I want the agent to open the web app in a browser and show me the result before declaring the task complete, so I can see visible changes without having to open the browser myself.

## Acceptance Criteria

- When an implementation task produces a browser-visible change (UI feature, frontend route, visual element, etc.), the agent MUST use the `chrome-devtools` MCP to open the app and show the result before marking the task done.
- The requirement is stated as MANDATORY (not optional/recommended) in the implementation prompt.
- The instruction is clear about what "browser-visible" means: anything a user would see at a URL.
- The instruction appears near the end of the implementation prompt, just before or as part of the "before declaring victory" / completion section.
- The instruction does not break non-UI tasks — it is conditional on the feature being browser-visible.
