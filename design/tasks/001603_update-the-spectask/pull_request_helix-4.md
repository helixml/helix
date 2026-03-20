# Add startup script info to planning prompt and log output to file

## Summary
Planning agents now know about the startup script location and where to read its log. The startup script output is also teed to `/tmp/helix-startup.log` so agents can inspect it.

## Changes
- `desktop/shared/helix-workspace-setup.sh`: Tee startup script output to `/tmp/helix-startup.log` using `${PIPESTATUS[0]}` to preserve exit code
- `api/pkg/services/spec_task_prompts.go`: Add "Startup Script" section to `planningPromptTemplate` with location and log path
