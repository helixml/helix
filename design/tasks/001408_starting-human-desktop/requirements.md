# Requirements: Desktop Startup Progress Indicators

## Problem Statement

Starting a human desktop session is slow (10-60 seconds) and the UI provides insufficient feedback during this time. Users see only a generic "Starting Desktop..." message with a spinner, giving no indication of actual progress or what's happening.

## User Stories

### US1: See meaningful progress during startup
**As a** user starting a desktop session  
**I want to** see what stage the startup is in  
**So that** I know it's making progress and roughly how long to wait

### US2: Understand delays
**As a** user waiting for a desktop  
**I want to** know if startup is taking longer than usual  
**So that** I can decide whether to wait or investigate

## Acceptance Criteria

### AC1: Stage-based progress messages
- [ ] Display distinct messages for each startup stage:
  - "Creating container..." (container creation)
  - "Unpacking build cache (X/Y GB)..." (golden cache copy, already exists)
  - "Starting desktop environment..." (compositor/DE init)
  - "Connecting..." (desktop-bridge registration)
- [ ] Messages update as stages complete

### AC2: Elapsed time indicator
- [ ] Show elapsed time after 5+ seconds (e.g., "Starting desktop environment... (12s)")
- [ ] Helps users calibrate expectations

### AC3: Works for both session types
- [ ] Progress shown in Kanban card screenshot mode (SpecTask sessions)
- [ ] Progress shown in floating window stream mode (Exploratory sessions)

## Out of Scope
- Estimated time remaining (too unreliable)
- Detailed sub-stage progress (e.g., individual Docker layers)
- Progress bar/percentage (stages have unpredictable durations)