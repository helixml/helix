# PDF export invalid hook call

## Root cause

`ToPDFImpl` is loaded with `React.lazy`, so Vite did not discover
`@uiw/react-md-editor` or `@react-pdf/renderer` during its initial dependency
optimization pass. Opening PDF export caused Vite to optimize both packages and
invalidate the dependency cache. The already-rendered application and the new
PDF chunk then referenced different cache generations, producing React's
invalid-hook-call error.

Vite attempted to reload the page after re-optimization, but the API frontend
proxy wrapped its response writer in `statusRecorder`, which did not preserve
`http.Hijacker`. The HMR WebSocket upgrade therefore failed before the reload
could reach the browser.

## Fix

- Pre-optimize both lazy PDF dependencies and deduplicate React/React DOM.
- Preserve `http.Hijacker` through `statusRecorder` so WebSocket upgrades work.

## Verification

- Opened Export to PDF in a live local org-chat session and waited for the PDF
  preview iframe; no hook exceptions occurred.
- Confirmed the proxied Vite HMR endpoint returns `101 Switching Protocols` and
  a connected frame.
- `go test ./api/pkg/server/spa -count=1`
- `go build ./api/pkg/server/ ./api/pkg/store/ ./api/pkg/types/`
- `cd frontend && yarn build`
- `cd frontend && yarn test src` (49 files, 298 tests passed; one skipped)
