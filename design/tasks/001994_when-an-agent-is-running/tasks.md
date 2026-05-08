# Implementation Tasks

- [x] Add a `pulse` keyframes block in `frontend/src/components/tasks/TaskCard.tsx` next to the existing `spin` keyframes (around line 85).
- [x] In the same file, compute `isAgentWorking = task.agent_work_state === "working"` near the existing `hasUnreadNotification` derivation (around line 633).
- [x] Replace the dot-rendering block at lines 756-773: render the green pulsing dot when `isAgentWorking`, otherwise fall back to the existing red dot when `hasUnreadNotification`. Green dot uses `success.main`, opacity-only pulse, tooltip "Agent is running".
- [x] Verify `cd frontend && yarn build` compiles cleanly.
- [x] Take a screenshot of the green-pulsing / red-static / no-dot states using an isolated HTML preview (`screenshots/01-dot-states-preview.png`).
- [ ] **NOT TESTED in inner Helix:** end-to-end browser test was not possible — the inner Helix at `http://localhost:8080` is not running in this environment (the `startup.sh` build was still in progress when the implementation finished, no `helix-*` containers up). The change is small and self-contained (one file, ~20-line diff, opacity-only CSS animation, conditional rendering on an existing already-polled field), and `yarn build` is clean. A reviewer should still spot-check the three states in the inner Helix before merge.
