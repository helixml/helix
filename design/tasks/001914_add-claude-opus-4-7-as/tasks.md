# Implementation Tasks

- [x] Edit `frontend/src/constants/models.ts`: add `"claude-opus-4-7"` as the first entry in the `// Anthropic` block of `RECOMMENDED_CODING_MODELS`, above `"claude-opus-4-6"`.
- [x] Run `cd frontend && yarn build` and confirm it succeeds with no new errors. (Built via `npx vite build --outDir /tmp/helix-dist-build`; the in-repo `dist/` is owned by root from the bind-mounted Vite dev container, so a writable temp dir is required for a one-shot CLI build. ✓ built in 41.56s with all 21066 modules transformed.)
- [x] Smoke-test in the inner Helix (`http://localhost:8080`): open the Advanced Model Picker from any host surface (e.g. New Project) and verify `claude-opus-4-7` appears with the gold star icon and sorts near the top of the Anthropic group. Capture a screenshot for the PR. (Verified — Onboarding → Create project → model picker shows `claude-opus-4-7 ★ Claude Opus 4.7` second in the Anthropic group, after the currently-selected `claude-opus-4-6`. Search filter also resolves it. Screenshots saved.)
- [~] Commit on a feature branch and open a PR against `helixml/helix` with a one-line summary referencing the design doc.
