# Implementation Tasks

- [ ] Add conventional commit format rule to CLAUDE.md "Commits & Debugging" section with type list and examples
- [ ] Update `commit-msg` hook to validate conventional commit format before adding Spec-Ref trailer (reject non-conforming messages with a helpful error)
- [ ] Update example commit messages in `agent_instruction_service.go` `approvalPromptTemplate` to use conventional format (e.g., `chore(specs): update progress` instead of `"Progress update"`)
- [ ] Verify the hook works: test with a bad commit message and confirm it rejects with a clear error, test with a valid conventional commit and confirm it passes
