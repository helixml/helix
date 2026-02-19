# Requirements: Onboarding Resolution Picker

## Overview

Add a desktop resolution option (1080p vs 4K) during onboarding when setting up the first agent.

## User Stories

1. **As a new user**, I want to choose my desktop resolution during onboarding so I can optimize for quality or parallelism from the start.

## Acceptance Criteria

- [ ] Resolution picker appears in onboarding Step 3 (project/agent creation) when creating a new agent
- [ ] Two options: **1080p** and **4K**
- [ ] Helper text explains the trade-off:
  - 1080p: "Run more agents in parallel"
  - 4K: "Sharper display quality"
- [ ] 4K automatically sets zoom to 200% (2x scaling)
- [ ] 1080p sets zoom to 100%
- [ ] Selected resolution is saved to `external_agent_config.resolution` on the created agent
- [ ] Default selection: 1080p (safer for most users)

## Out of Scope

- 5K resolution option (only shown in advanced agent settings)
- Desktop type selection (ubuntu/sway)
- Refresh rate configuration