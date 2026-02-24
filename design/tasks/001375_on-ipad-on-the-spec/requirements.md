# Requirements: iPad Text Selection & Comment Panel Improvements

## Problem Statement

On iPad, the spec review page has two usability issues:
1. Highlighting text with touch doesn't trigger the comment panel (only `onMouseUp` is handled, not touch events)
2. The comment panel sometimes obscures the text being commented on (hardcoded `left: 670px` positioning doesn't adapt to screen size)

## User Stories

### US-1: Touch-Based Text Selection
**As a** reviewer using an iPad  
**I want to** highlight text by touch and have the comment panel appear  
**So that** I can add inline comments without needing a mouse

### US-2: Readable Comment Context  
**As a** reviewer on any device  
**I want** the comment panel to not obscure the text I'm commenting on  
**So that** I can reference the original text while writing my comment

## Acceptance Criteria

### AC-1: Touch Selection Support
- [ ] Text selection via touch triggers `handleTextSelection` (same as mouse)
- [ ] Comment form appears at correct Y position relative to selection
- [ ] Works on iPad Safari and Chrome

### AC-2: Responsive Comment Panel Positioning
- [ ] On wide screens (>1000px): panel appears to the right of content (current behavior)
- [ ] On narrow screens (≤1000px): panel appears below the quoted text or as a modal/drawer
- [ ] Panel never overlaps the main content area on any screen size

## Out of Scope
- Desktop text selection behavior (unchanged)
- Comment functionality itself (unchanged)
- Other mobile browsers beyond iPad Safari/Chrome