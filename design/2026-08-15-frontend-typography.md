# Frontend typography — one place to change how Helix text looks

**Status:** implemented (branch `fix/chat-code-block-styling`)

## Where the settings live

`frontend/src/styles/typography.ts` is the **single source of truth**. It exports:

| Export | Purpose |
|---|---|
| `SANS_FONT_STACKS` | Selectable interface/prose stacks (`system`, `dmSans`) |
| `MONO_FONT_STACKS` | Selectable code stacks (`system`, `jetbrainsMono`) |
| `TYPOGRAPHY` | **The active config** — families, sizes, line heights, smoothing |
| `APP_FONT_FAMILY` / `APP_MONO_FONT_FAMILY` | Resolved stacks, for `sx`/styled usage |
| `typographyCssVariables()` | `--helix-font-*` custom properties for plain-CSS surfaces |

Changing a value in `TYPOGRAPHY` restyles the whole app. Nothing else should
hardcode a font family or a chat/code font size.

Consumers:

- `frontend/src/contexts/theme.tsx` — MUI `typography.fontFamily`,
  `typography.fontSize`, the `html` root size, `-webkit-font-smoothing`, and the
  `:root` CSS variables (via `MuiCssBaseline`).
- `frontend/src/components/session/Markdown.tsx` — chat prose size/line-height,
  inline-code size.
- `frontend/src/components/session/MarkdownCodeBlock.tsx` — code and chrome sizes.

## Current defaults (matching T3 Code)

| Token | Value | Notes |
|---|---|---|
| `sans` | `-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif` | Native OS UI font; no webfont download |
| `mono` | `"SF Mono", "SFMono-Regular", Menlo, Consolas, "Liberation Mono", monospace` | Concrete names first — some engines alias `ui-monospace` to the *proportional* system font |
| `rootFontSize` | 16px | Every rem-based dimension scales with it |
| `bodyFontSize` | 14 | MUI baseline for body/button/caption |
| `chatFontSize` / `chatLineHeight` | `0.875rem` / `1.625` | T3's `text-sm leading-relaxed` |
| `codeFontSize` / `codeLineHeight` | 13px / 1.55 | px so it never scales twice |
| `inlineCodeFontSize` | `0.75rem` | Stays under the sentence rather than towering over it |
| `smoothing` | `true` | Grayscale `antialiased`; macOS engines only |

Helix previously defaulted to DM Sans (geometric, wider, larger x-height), which
is what made our chat read visibly heavier than T3's.

## How to change a font

1. Edit `TYPOGRAPHY` in `frontend/src/styles/typography.ts`.
2. If you select a **webfont** preset (e.g. `SANS_FONT_STACKS.dmSans`), keep the
   matching `@fontsource-variable/*` import in `frontend/src/index.tsx` — the
   preset falls back to the system font on machines without it installed.
3. `cd frontend && yarn build` (or rely on Vite HMR in dev).

To add a preset, add it to `SANS_FONT_STACKS` / `MONO_FONT_STACKS` and point
`TYPOGRAPHY.sans` / `.mono` at it. Do not add a font-family literal to a
component.

## Rules for agents

- **Never** write a `fontFamily:` literal in a component. Import
  `APP_FONT_FAMILY` / `APP_MONO_FONT_FAMILY`, or use `var(--helix-font-sans)` /
  `var(--helix-font-mono)` in plain CSS.
- **Never** hardcode chat prose or code font sizes. Use the `TYPOGRAPHY.*`
  tokens so all chat surfaces stay in step.
- One exception exists on purpose: the Helix wordmark in
  `ChatSidebarBrandHeader.tsx` pins Sora. Brand marks are not body text.

## Chat code blocks

Related, same branch: `MarkdownCodeBlock` follows T3's single-surface treatment
— one flat panel, a transparent header (no tint, no divider), and a highlighted
`<pre><code>` with **no background of its own**.

The bug this fixed: the highlighter rendered with `PreTag="div"`, so the emitted
`<code>` matched the `:not(pre) > code` inline-code rule in `Markdown.tsx` and
picked up the inline-code background, border, padding and radius. Because
`<code>` is inline, that background painted per line box — the ragged boxes
hugging each line inside the block. Rendering a real `<pre>` wrapper (plus
explicit `pre > code` resets) is the fix; `setupTests.ts` now mocks
`react-syntax-highlighter` with a real `<pre><code>` wrapper so a regression
fails the test instead of rendering wrong.
