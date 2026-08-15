/**
 * App typography tokens — the single place to change how Helix text looks.
 *
 * Everything (MUI theme, chat markdown, code blocks, plain CSS surfaces) reads
 * from `TYPOGRAPHY` below, either as a TS import or via the `--helix-font-*`
 * CSS variables emitted by `typographyCssVariables()` in `contexts/theme.tsx`.
 * To restyle the app, edit `TYPOGRAPHY` — do not hardcode families or sizes in
 * components.
 *
 * Defaults track the T3 Code chat surface: the native OS UI font at 16px root
 * / 14px prose, a concrete monospace stack at 13px, and grayscale smoothing.
 *
 * See `design/2026-08-15-frontend-typography.md` for the rationale and how to
 * change a token.
 */

/**
 * Sans presets. `system` needs no webfont; the others require their
 * `@fontsource-variable/*` import in `src/index.tsx` to be kept.
 */
export const SANS_FONT_STACKS = {
  /** Native UI font of the host OS (SF on macOS, Segoe on Windows, the
   *  desktop font on Linux). Matches T3 Code. */
  system: '-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif',
  /** Helix's previous default — geometric, wider, noticeably larger x-height. */
  dmSans:
    '"DM Sans Variable", "DM Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif',
} as const

/**
 * Mono presets. Concrete family names come first on purpose: some engines
 * alias `ui-monospace` to the proportional system UI font, which breaks every
 * code surface.
 */
export const MONO_FONT_STACKS = {
  system: '"SF Mono", "SFMono-Regular", Menlo, Consolas, "Liberation Mono", monospace',
  jetbrainsMono: '"JetBrains Mono Variable", "SFMono-Regular", Menlo, Consolas, monospace',
} as const

/**
 * The active configuration. Change a value here and the whole app follows.
 */
export const TYPOGRAPHY = {
  /** Interface + prose font. */
  sans: SANS_FONT_STACKS.system,
  /** Code blocks, inline code, terminals, ids and other fixed-width text. */
  mono: MONO_FONT_STACKS.system,

  /** Root (html) size in px — every rem-based dimension scales with it. */
  rootFontSize: 16,
  /** MUI `typography.fontSize`: the baseline for body/button/caption variants. */
  bodyFontSize: 14,
  /** Chat prose (assistant + user markdown). */
  chatFontSize: '0.875rem',
  chatLineHeight: 1.625,
  /** Code inside markdown code blocks, in px so it never scales twice. */
  codeFontSize: 13,
  codeLineHeight: 1.55,
  /** Inline `code` spans stay under their sentence rather than towering over it. */
  inlineCodeFontSize: '0.75rem',
  /** Code-block chrome: the language label and toolbar. */
  codeChromeFontSize: '0.6875rem',

  /**
   * Grayscale antialiasing (thinner strokes) instead of the heavier platform
   * default. Only macOS engines honour it. `false` restores stem darkening.
   */
  smoothing: true,
} as const

/** Resolved sans stack. Prefer this over a literal font-family anywhere. */
export const APP_FONT_FAMILY = TYPOGRAPHY.sans

/** Resolved mono stack. Prefer this over a literal font-family anywhere. */
export const APP_MONO_FONT_FAMILY = TYPOGRAPHY.mono

/**
 * CSS custom properties for surfaces that cannot import TS (plain CSS, shadow
 * roots, embedded widgets). Applied to `:root` by the theme provider.
 */
export const typographyCssVariables = (): Record<string, string> => ({
  '--helix-font-sans': TYPOGRAPHY.sans,
  '--helix-font-mono': TYPOGRAPHY.mono,
  '--helix-font-size-root': `${TYPOGRAPHY.rootFontSize}px`,
  '--helix-font-size-body': `${TYPOGRAPHY.bodyFontSize}px`,
  '--helix-font-size-chat': TYPOGRAPHY.chatFontSize,
  '--helix-font-size-code': `${TYPOGRAPHY.codeFontSize}px`,
  '--helix-line-height-chat': `${TYPOGRAPHY.chatLineHeight}`,
})
