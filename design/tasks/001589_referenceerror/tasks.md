# Implementation Tasks

- [x] Add `TypesInteractionState` to the import in `frontend/src/contexts/streaming.tsx` line 21
- [x] Run `cd frontend && yarn build` to verify no build errors (verified via `npx tsc --noEmit -p tsconfig.json` — clean)
- [x] Fix CI tsc command in `.drone.yml` (`yarn tsc --noEmit` is invalid with `-b`; replaced with `npx tsc --noEmit -p tsconfig.json`)
