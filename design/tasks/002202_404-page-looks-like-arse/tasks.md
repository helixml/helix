# Implementation Tasks: Fix 404 Page Legibility in Light Mode

- [x] Import and call `useLightTheme()` in `frontend/src/pages/NotFound.tsx` to get `isLight`, `textColor`, `textColorFaded`, `border`, and `highlightColor`.
- [x] Replace the hardcoded "404" glyph color (line 111) with a mode-aware value: keep `rgba(255,255,255,0.08)` in dark, use a faint dark tint (e.g. `rgba(0,0,0,0.08)`) in light.
- [x] Replace the heading color (line 133) with a mode-aware near-black/light-gray (`textColor`).
- [x] Replace the body text color (line 142) with `textColorFaded`.
- [x] Make the outlined "Organizations" button mode-aware: border (`border`), text (`textColorFaded`/`textColor`), and hover accent (dark `#00E5FF`, light `highlightColor`/`#0e7490`).
- [x] Make the text "Go back" button mode-aware: base text (`textColorFaded`), hover text, and hover background tint branched by mode.
- [x] Adjust the background radial gradient so it does not look broken/lone-cyan in light mode (optional low-priority tint branch).
- [x] Verify the contained "Home" button (cyan bg + black text) is left unchanged — it works in both modes.
- [x] Confirm dark mode renders identically to the current version (colors, animations, layout).
- [x] `cd frontend && yarn build` to confirm it compiles.
- [x] Test in the inner Helix: render the 404 page, toggle light/dark, confirm legibility and no dark-mode regression; capture before/after screenshots.
