# Implementation Tasks

- [x] Add a `pulse` keyframes block in `frontend/src/components/tasks/TaskCard.tsx` next to the existing `spin` keyframes (around line 85).
- [x] In the same file, compute `isAgentWorking = task.agent_work_state === "working"` near the existing `hasUnreadNotification` derivation (around line 633).
- [x] Replace the dot-rendering block at lines 756-773: render the green pulsing dot when `isAgentWorking`, otherwise fall back to the existing red dot when `hasUnreadNotification`. Green dot uses `success.main`, opacity-only pulse, tooltip "Agent is running".
- [x] Verify `cd frontend && yarn build` compiles cleanly.
- [ ] Manual test in the inner Helix at http://localhost:8080: start a task and confirm the green dot pulses while the agent is working.
- [ ] Manual test: let an agent finish (red dot appears), then trigger a new run via follow-up prompt — confirm the red dot is replaced by the green pulsing dot, and the red dot returns once the agent goes idle again.
- [ ] Confirm no regression on the existing red-dot-on-idle behaviour (agent finishes, no new run, click dismisses as before).
- [ ] Take a screenshot of each state (working / idle-with-attention / working-while-stale-attention) and attach to the PR.
