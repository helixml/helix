# Implementation Tasks

- [ ] In `helix-4/api/pkg/services/agent_instruction_service.go`, add a mandatory "Before Declaring Done" subsection to `approvalPromptTemplate` after the existing Visual Testing section, requiring agents to use `chrome-devtools` MCP to open and show the app when the feature is browser-visible
- [ ] Change the framing of the final screenshot step from "optional but valuable" to mandatory for browser-visible features (keep it optional for exploration/planning screenshots)
- [ ] Build and verify: `cd helix-4/api && go build ./...`
- [ ] Push changes to a feature branch and open a PR
