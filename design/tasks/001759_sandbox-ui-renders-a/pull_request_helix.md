# Fix sandbox UI blank page caused by crypto.randomUUID in HTTP context

## Summary
Replace `crypto.randomUUID()` with `uuidv4()` from the `uuid` npm package in `DesktopStreamViewer.tsx`. The Web Crypto API's `randomUUID()` only works in secure contexts (HTTPS or localhost), so accessing the sandbox over plain HTTP throws `Uncaught TypeError: crypto.randomUUID is not a function`, crashing the React tree to a blank page.

## Changes
- Added `import { v4 as uuidv4 } from 'uuid'` to `DesktopStreamViewer.tsx`
- Replaced `crypto.randomUUID()` with `uuidv4()` in `getOrCreateStreamUUID()`
- No new dependencies — `uuid` v11.0.5 is already in `package.json` and used in 4 other files
