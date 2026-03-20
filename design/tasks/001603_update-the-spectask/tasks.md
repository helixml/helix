# Implementation Tasks

- [x] Update `helix-workspace-setup.sh` to tee startup script output to `/tmp/helix-startup.log` (use `${PIPESTATUS[0]}` to preserve exit code)
- [~] Add "Startup Script" section to `planningPromptTemplate` in `api/pkg/services/spec_task_prompts.go`
- [ ] Verify the Go template compiles (`go build ./api/pkg/services/...`)
