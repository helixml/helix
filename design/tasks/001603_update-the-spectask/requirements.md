# Requirements: Update Spectask Prompt with Startup Script Info

## User Story

As a spec task planning agent, I need to know about the project startup script and where to find its log, so I can understand the project environment and debug any startup issues during planning.

## Background

The project startup script lives at `/home/retro/work/helix-specs/.helix/startup.sh`. It runs automatically when the desktop session starts (via `helix-workspace-setup.sh`), installing dependencies and starting dev servers. The script output currently goes to the "Helix Setup" terminal window but is not captured in a file the agent can read.

The current planning prompt (`planningPromptTemplate` in `api/pkg/services/spec_task_prompts.go`) has no mention of the startup script or its log.

## Acceptance Criteria

1. The planning prompt includes the location of the startup script: `/home/retro/work/helix-specs/.helix/startup.sh`
2. The planning prompt tells the agent it can read the startup script log at a specific path (the implementer must determine or create this log path)
3. The startup script output is redirected to a log file so it can be read by the agent
4. The planning prompt section fits naturally with the existing prompt structure (after the Git workflow section or before "Document Your Learnings")
