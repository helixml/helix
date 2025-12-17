/**
 * Browser locale detection hook for keyboard layout auto-configuration.
 *
 * Detects the user's browser language/locale and derives the appropriate
 * XKB keyboard layout code for use in desktop containers.
 *
 * @see design/2025-12-17-keyboard-layout-option.md
 */

export interface BrowserLocaleInfo {
  /** Full browser language tag, e.g., "en-US", "fr-FR" */
  language: string;
  /** XKB keyboard layout code, e.g., "us", "fr", "de" */
  keyboardLayout: string;
  /** IANA timezone, e.g., "America/New_York" */
  timezone: string;
}

/**
 * Derives an XKB keyboard layout code from a browser language tag.
 *
 * Maps BCP 47 language tags (e.g., "fr-FR") to XKB layout codes (e.g., "fr").
 * Handles special cases where region matters (en-GB vs en-US, pt-BR vs pt-PT).
 *
 * @param language - BCP 47 language tag from navigator.language
 * @returns XKB keyboard layout code
 */
function deriveKeyboardLayout(language: string): string {
  const langCode = language.split('-')[0].toLowerCase();  // "fr-FR" → "fr"
  const regionCode = language.split('-')[1]?.toLowerCase();  // "fr-FR" → "fr"

  // Special cases where region determines keyboard layout
  if (langCode === 'en') {
    // British English uses different keyboard layout than US
    if (regionCode === 'gb' || regionCode === 'uk') return 'gb';
    return 'us';
  }

  if (langCode === 'pt') {
    // Brazilian Portuguese uses different keyboard layout than Portuguese
    if (regionCode === 'br') return 'br';
    return 'pt';
  }

  // Chinese and Japanese use different XKB codes than their language codes
  if (langCode === 'zh') return 'cn';
  if (langCode === 'ja') return 'jp';

  // Korean
  if (langCode === 'ko') return 'kr';

  // Default: use language code directly as layout code
  // This works for: fr, de, es, it, ru, pl, nl, se, no, dk, fi, etc.
  return langCode;
}

/**
 * Hook to get browser locale information for keyboard layout configuration.
 *
 * Returns the browser's language preference, derived keyboard layout,
 * and timezone. Used to automatically configure keyboard layouts in
 * desktop containers to match the user's local setup.
 *
 * @example
 * ```tsx
 * const { keyboardLayout, timezone } = useBrowserLocale();
 * // keyboardLayout: "fr" for French browser
 * // timezone: "Europe/Paris"
 * ```
 */
export function useBrowserLocale(): BrowserLocaleInfo {
  const language = navigator.language || 'en-US';
  const keyboardLayout = deriveKeyboardLayout(language);
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';

  return { language, keyboardLayout, timezone };
}

/**
 * Non-hook version for use outside React components.
 * Same logic as useBrowserLocale but can be called anywhere.
 */
export function getBrowserLocale(): BrowserLocaleInfo {
  const language = navigator.language || 'en-US';
  const keyboardLayout = deriveKeyboardLayout(language);
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';

  return { language, keyboardLayout, timezone };
}
