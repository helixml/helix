# Design

## Location of Change

The implementation prompt template is in:

**`helix-4/api/pkg/services/agent_instruction_service.go`**

Specifically, `approvalPromptTemplate` (line ~91). This is the prompt sent to agents when their spec is approved and they enter the implementation phase.

The file also contains `implementationReviewPromptTemplate` (line ~330), which is sent when the agent's code is pushed for review. That template already says "If this is a web app, please start the dev server and provide the URL" — but this is too weak and too late.

## Decision

Add a **mandatory** instruction block to `approvalPromptTemplate`, integrated into or just after the existing "Visual Testing & Screenshots" section (lines ~157-183). The existing section is marked "optional but valuable" — we need to strengthen it to mandatory for browser-visible features.

The new block should:
1. State clearly: **if the feature is visible in a browser, you MUST use chrome-devtools MCP to open the app and show it before declaring done**
2. Give a concrete, minimal workflow (navigate → screenshot or just show the page)
3. Be conditional ("if browser-visible") so it doesn't apply to pure backend/non-UI tasks

## Key Design Notes

- The existing Visual Testing section says "optional but valuable" — we change the framing for the completion requirement to mandatory
- The `chrome-devtools` MCP is already documented in the prompt; we're adding a completion gate, not new tooling
- Pattern found: `implementationReviewPromptTemplate` already nudges agents to start dev server — the new instruction in `approvalPromptTemplate` adds the show-it requirement earlier, while the agent is still working
- The prompt template uses Go `text/template` with `{{.TaskDirName}}` etc. — plain markdown text additions require no template changes

## Approach

Strengthen the existing "Visual Testing & Screenshots" section from optional to:
- **Optional** for planning/exploration screenshots (keep existing text)
- **Mandatory** for the final "before declaring done" step when the feature is browser-visible

Add a new subsection at the end of the Visual Testing block:

```
## Before Declaring Done (Browser-Visible Features)

If your feature or change is visible in a browser:

1. Use the `chrome-devtools` MCP to open the web app
2. Navigate to the relevant page/feature
3. Take a screenshot and save it to the task's screenshots/ folder
4. Show it to the user in your completion message

Do NOT declare the task complete without doing this — the user needs to see it working.
```
