# Implementation Tasks

- [~] In `frontend/src/components/external-agent/DesktopStreamViewer.tsx`, add `import { v4 as uuidv4 } from 'uuid'`
- [~] Replace `crypto.randomUUID()` (line 115) with `uuidv4()`
- [ ] Run `cd frontend && yarn build` to verify no build errors
- [ ] Test sandbox UI loads over HTTP without blank page
