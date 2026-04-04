# Requirements: Fix Startup Script Button & Planning Task Status Indicators

## User Stories

### Story 1: Startup Script Fix Button Should Be Just Do It
**As a** user testing my startup script in project settings,  
**I want** the "Get AI to fix it" button to start a "just do it" task,  
**So that** the AI immediately fixes the script without writing specs first.

**Current Behavior:** Button creates a regular spec task that enters planning phase. UI appears stuck because there's no indication the agent is working on specs.

**Acceptance Criteria:**
- [ ] "Get AI to fix it" button creates task with `just_do_it_mode: true`
- [ ] Task goes directly to `queued_implementation` status, skipping spec generation

### Story 2: Visual Indicator for Planning Tasks Awaiting Specs
**As a** user with tasks in planning phase,  
**I want** a visual indicator showing the agent is working on specs,  
**So that** I know the task isn't stuck and understand what I'm waiting for.

**Current Behavior:** Tasks in `spec_generation` status show no indication that the agent needs to push specs. Similar to how PR phase shows "Waiting for agent to push branch..." but planning phase has no equivalent.

**Acceptance Criteria:**
- [ ] Tasks in `spec_generation` status show "Waiting for agent to push specs..." message
- [ ] After 30 seconds, show warning: "Agent hasn't pushed specs yet. Please check if the agent is having trouble."
- [ ] Visual style matches existing PR waiting indicator pattern

### Story 3: Skip Spec Button (Optional)
**As a** user with a planning task that I want to implement immediately,  
**I want** a "Skip Spec" button (non-primary styling),  
**So that** I can move the task to implementation without waiting for specs.

**Acceptance Criteria:**
- [ ] "Skip Spec" button appears for tasks in planning phase
- [ ] Button uses non-primary styling (outlined or text, not contained)
- [ ] Clicking moves task directly to `queued_implementation` (or `implementation`)
- [ ] Task remains functional - user can still open PR later

### Story 4: Return to Backlog Button for Reviewed Specs (Optional)
**As a** user with finished specs in the review column,  
**I want** a "Return to Backlog" button (non-primary styling),  
**So that** I can move the task back if I want to re-spec for a different repo or change direction.

**Acceptance Criteria:**
- [ ] "Return to Backlog" button appears for tasks in `spec_review` status
- [ ] Button uses non-primary styling (outlined or text, not contained)
- [ ] Clicking moves task back to `backlog` status
- [ ] User can then re-start planning with different parameters if needed
