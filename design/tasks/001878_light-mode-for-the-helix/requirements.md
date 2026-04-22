# Requirements: Light Mode for the Helix Frontend

## Overview

Add a user-toggleable light mode to the Helix frontend. The codebase already has partial infrastructure (theme context with `toggleMode()`, light/dark color pairs in `themes.tsx`, `useLightTheme` hook) but the mode is hardcoded to dark and no toggle UI exists.

## User Stories

### US-1: Toggle Light/Dark Mode
**As a** user, **I want to** switch between light and dark mode **so that** I can use Helix comfortably in different lighting conditions.

**Acceptance Criteria:**
- A toggle control is visible in the app (settings or top bar)
- Clicking the toggle immediately switches the entire UI between light and dark themes
- The selected mode persists across page reloads (localStorage)
- The app respects the user's OS-level color scheme preference on first visit (`prefers-color-scheme`)

### US-2: Consistent Light Theme
**As a** user in light mode, **I want** all UI elements to use appropriate light-mode colors **so that** the interface is readable and visually cohesive.

**Acceptance Criteria:**
- Backgrounds are light (#ffffff or similar), text is dark (#333 or similar)
- All dialogs, menus, popovers, tooltips, and panels use light-appropriate colors
- Scrollbar styling adapts to light mode
- Charts and data visualizations remain readable in light mode
- No hardcoded dark-mode colors leak through (no dark backgrounds with dark text, etc.)

### US-3: MUI Component Overrides
**As a** user in light mode, **I want** MUI components (Dialog, Menu, Paper, Popover) to render with light-appropriate styles **so that** they don't appear as dark patches in a light UI.

**Acceptance Criteria:**
- MuiMenu uses light backgrounds with dark text
- MuiDialog uses light paper background
- MuiPaper/Popover surfaces use light colors
- Backdrop blur and shadows remain subtle and appropriate

## Out of Scope

- Custom user-defined color themes beyond light/dark
- Per-page or per-component theme overrides
- Server-side theme persistence (localStorage is sufficient)
- Redesigning the overall layout or component structure
