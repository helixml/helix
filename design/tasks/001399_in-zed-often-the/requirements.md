# Requirements: Prevent Keyboard Focus Stealing When Following Agent

## Problem Statement

When the "follow agent" feature is active (even implicitly), keyboard focus jumps to text editors that the agent opens. This causes the user's keystrokes to be typed into code files instead of the prompt input, resulting in:
- Random characters appearing in source code files
- Lost prompt text
- Potential commits of garbage code

## User Stories

### US-1: User typing prompt while agent works
**As a** user typing a prompt in the agent panel  
**I want** my keyboard focus to stay in the prompt input  
**So that** my keystrokes don't accidentally modify source files

### US-2: User reviewing code while agent works
**As a** user reading code in an editor while the agent works  
**I want** my cursor position and focus to remain stable  
**So that** I can continue reading without interruption

### US-3: User explicitly navigating to agent's location
**As a** user who wants to see what the agent is doing  
**I want** to click to navigate to the agent's current file  
**So that** I can follow along when I choose to

## Acceptance Criteria

### AC-1: No automatic focus transfer
- [ ] When the agent opens a file, keyboard focus remains where it was
- [ ] When the agent navigates to a different position, keyboard focus is not transferred
- [ ] The editor pane can still visually update to show the agent's location without taking focus

### AC-2: Explicit user actions still work
- [ ] Clicking on an editor tab focuses that editor
- [ ] Clicking in an editor focuses that editor
- [ ] Keyboard navigation (e.g., `cmd+tab`) still moves focus normally

### AC-3: Follow toggle button behavior
- [ ] The "follow agent" toggle should control visual tracking (which file is shown), not keyboard focus
- [ ] Visual indication of follow state should match actual behavior

## Out of Scope

- Changing the overall "follow" semantics for collaborative editing (peer-to-peer following)
- Modifying how the agent panel itself handles focus