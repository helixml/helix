# Design

## Change

Replace `crypto.randomUUID()` with `uuidv4()` from the `uuid` npm package in `DesktopStreamViewer.tsx`.

## Why `uuid` package over other options

The codebase already has two UUID generation mechanisms:
1. **`uuid` npm package (v11.0.5)** — used in 4 other files (`useInteractionQuestions.ts`, `QuestionSetDialog.tsx`, `ApiIntegrations.tsx`, `FineTuneTextQuestionEditor.tsx`)
2. **`utils/instanceId.ts`** — custom `Math.random()`-based UUID generator

The `uuid` package is the right choice because:
- Already the established pattern in this codebase
- Cryptographically stronger than the `Math.random()` fallback in `instanceId.ts`
- Works in all browser contexts (HTTP, HTTPS, localhost)

## Affected File

**`frontend/src/components/external-agent/DesktopStreamViewer.tsx`**
- Line 115: Replace `crypto.randomUUID()` with `uuidv4()`
- Add import: `import { v4 as uuidv4 } from 'uuid'`

## Codebase Notes

- The `uuid` package and `@types/uuid` are already in `frontend/package.json` — no new dependency needed
- The `getOrCreateStreamUUID()` function is a module-level helper (not a component), so the import is straightforward
- `ErrorBoundary` in `index.tsx` catches render errors but this crash leaves the page blank rather than showing a useful error
