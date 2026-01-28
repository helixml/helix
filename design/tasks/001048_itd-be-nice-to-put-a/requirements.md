# Requirements: Stop/Start Buttons for Agent Spec Task Page

## Overview

Add stop and start buttons alongside the existing restart button in the SpecTask detail page, allowing users to stop a desktop environment to free up resources and resume it later with the same persisted session.

## User Stories

### US1: Stop Desktop Session
**As a** user working on a spec task  
**I want to** stop the desktop environment without restarting it  
**So that** I can free up resources when taking a break and resume later

**Acceptance Criteria:**
- Stop button appears next to restart button when desktop is running
- Clicking stop shows brief confirmation or stops immediately
- Desktop transitions to "paused" state after stopping
- Session data persists (no data loss)

### US2: Start Paused Desktop
**As a** user returning to a paused spec task  
**I want to** start the desktop from the toolbar buttons  
**So that** I can resume work without hunting for the play button in the video area

**Acceptance Criteria:**
- Play/Start button appears in toolbar when desktop is stopped
- Restart button is hidden when desktop is stopped (doesn't make sense)
- Clicking start resumes the same session
- Chat history remains accessible throughout

### US3: Consistent UI State
**As a** user  
**I want** the toolbar buttons to reflect the desktop state  
**So that** I always know what actions are available

**Acceptance Criteria:**
- Running state: Shows Stop + Restart buttons
- Stopped/Paused state: Shows Start button only (no Restart)
- Starting state: Shows disabled spinner state
- Buttons update promptly when state changes

## Out of Scope

- Chat panel bugs (addressed separately)
- Changes to the video stream overlay start button (already exists)
- Session persistence fixes (assumed working)