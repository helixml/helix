# Implementation Tasks

- [x] In `frontend/src/components/spec-tasks/ReviewActionFooter.tsx`, wrap the "Approve Design" `<Button>` (the `else` branch of the `hasNextDocument && unresolvedCount === 0` ternary, ~lines 106–114) in a `<Tooltip>` with `title={unresolvedCount > 0 ? \`Resolve ${unresolvedCount} comment${unresolvedCount !== 1 ? 's' : ''} before approving\` : ''}` and `placement="top"`, using the `<Tooltip><span><Button disabled>...</Button></span></Tooltip>` pattern (mirror the existing "Start Implementation" tooltip in the same file).
- [x] Run `cd frontend && yarn tsc` — must pass with no new errors.
- [x] Run `cd frontend && yarn build` — must complete cleanly.
- [~] Test in inner Helix at `http://localhost:8080`: register/login (`test@helix.ml` / `helixtest`), open a spec task in the design-review state, verify scenarios 1–5 from `design.md` § Test plan (especially: with unresolved comments, hover the disabled "Approve Design" button and confirm the tooltip appears with correct count + plural).
- [ ] Capture before/after screenshots of scenario 3 (pending comments, hover state) and save to `screenshots/` in this task directory.
- [ ] Commit with `Spec-Ref: helix-specs@<sha>:002001_restore-tooltip-on-next` trailer; open PR against `helixml/helix:main` linking back to the spec docs and PR 2364.
