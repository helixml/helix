# Implementation Tasks

- [x] Edit `frontend/src/constants/models.ts`: add `"claude-opus-4-7"` as the first entry in the `// Anthropic` block of `RECOMMENDED_CODING_MODELS`, above `"claude-opus-4-6"`.
- [~] Run `cd frontend && yarn build` and confirm it succeeds with no new errors.
- [ ] Smoke-test in the inner Helix (`http://localhost:8080`): open the Advanced Model Picker from any host surface (e.g. New Project) and verify `claude-opus-4-7` appears with the gold star icon and sorts near the top of the Anthropic group. Capture a screenshot for the PR.
- [ ] Commit on a feature branch and open a PR against `helixml/helix` with a one-line summary referencing the design doc.
