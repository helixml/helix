# Requirements: Port Exposure UI

## Overview

Add a UI to expose ports from dev containers in Helix sessions. The backend API already exists from the original Hydra implementation—this task adds the frontend interface.

## User Stories

### US1: Expose a Port
As a developer running a web app in my dev container, I want to expose a port (e.g., 3000) so I can test it on my phone or another device.

**Acceptance Criteria:**
- Button in the desktop stream toolbar to open port exposure dialog
- Dialog lets me enter port number and optional name
- After exposing, I see the public URL(s) I can use to access it
- URL is copyable to clipboard

### US2: View Exposed Ports
As a developer, I want to see which ports are currently exposed for my session.

**Acceptance Criteria:**
- List of currently exposed ports with their URLs
- Each port shows: port number, name (if set), URL, status

### US3: Unexpose a Port
As a developer, I want to remove access to a port I previously exposed.

**Acceptance Criteria:**
- Delete/remove button next to each exposed port
- Confirmation before removing (or instant with undo snackbar)

## API Endpoints (Already Exist)

- `GET /api/v1/sessions/{id}/expose` - List exposed ports
- `POST /api/v1/sessions/{id}/expose` - Expose a port (body: `{ port, protocol?, name? }`)
- `DELETE /api/v1/sessions/{id}/expose/{port}` - Unexpose a port

## Out of Scope

- TCP protocol support (HTTP only for MVP)
- Authentication for exposed URLs
- Custom domains