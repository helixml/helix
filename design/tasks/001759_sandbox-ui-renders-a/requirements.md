# Requirements

## Problem

The sandbox UI renders a blank page due to a frontend runtime error:

```
Uncaught TypeError: crypto.randomUUID is not a function
```

`crypto.randomUUID()` is a Web Crypto API that is only available in **secure contexts** (HTTPS or localhost). When the sandbox is accessed over plain HTTP on a non-localhost address, the function is `undefined` and the call throws, crashing the React tree and leaving a blank page.

## Root Cause

`DesktopStreamViewer.tsx:115` calls `crypto.randomUUID()` inside `getOrCreateStreamUUID()`. This function generates a per-tab UUID for desktop streaming sessions. The rest of the codebase uses the `uuid` npm package (`v4 as uuidv4`) for UUID generation, which works in all contexts.

## User Stories

- **As a user**, I want the sandbox UI to load correctly over HTTP so I can use the sandbox without HTTPS.

## Acceptance Criteria

- [ ] Sandbox UI loads without errors when accessed over plain HTTP (non-localhost)
- [ ] `crypto.randomUUID` is no longer used anywhere in the frontend
- [ ] The replacement uses the `uuid` npm package (already a project dependency) for consistency with the rest of the codebase
- [ ] Desktop stream viewer continues to generate unique per-tab UUIDs for session differentiation
