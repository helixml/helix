/**
 * Browser locale detection hook for keyboard layout auto-configuration.
 *
 * Detects the user's browser language/locale and derives the appropriate
 * XKB keyboard layout code for use in desktop containers.
 *
 * Supports a `?keyboard=XX` query parameter override for testing different
 * layouts without changing browser settings.
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
  /** Whether the keyboard layout was overridden via query param */
  isOverridden: boolean;
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
 * Valid XKB layout codes that can be used as overrides.
 * This prevents injection of arbitrary values via query params.
 */
const VALID_LAYOUTS = new Set([
  'us', 'gb', 'fr', 'de', 'es', 'it', 'pt', 'br', 'ru', 'jp', 'cn', 'kr',
  'nl', 'se', 'no', 'dk', 'fi', 'pl', 'cz', 'hu', 'ro', 'bg', 'ua', 'tr',
  'gr', 'il', 'ar', 'th', 'vn', 'in', 'ch', 'be', 'at', 'ie', 'au', 'nz',
]);

/**
 * Gets keyboard layout override from URL query parameter.
 * Returns null if no valid override is present.
 *
 * @example
 * // URL: /specs/123?keyboard=fr
 * getKeyboardOverride() // returns "fr"
 */
function getKeyboardOverride(): string | null {
  if (typeof window === 'undefined') return null;

  const params = new URLSearchParams(window.location.search);
  const override = params.get('keyboard')?.toLowerCase();

  if (override) {
    if (VALID_LAYOUTS.has(override)) {
      console.log(`%c[Keyboard Override] Using layout from URL: ${override}`, 'color: #4CAF50; font-weight: bold; font-size: 14px;');
      console.log(`[Keyboard Override] Full URL: ${window.location.href}`);
      return override;
    } else {
      console.warn(`[Keyboard Override] Invalid layout "${override}" - not in allowed list. Using browser default.`);
    }
  }

  return null;
}

/**
 * Hook to get browser locale information for keyboard layout configuration.
 *
 * Returns the browser's language preference, derived keyboard layout,
 * and timezone. Used to automatically configure keyboard layouts in
 * desktop containers to match the user's local setup.
 *
 * Supports `?keyboard=XX` query parameter to override detected layout for testing.
 *
 * @example
 * ```tsx
 * const { keyboardLayout, timezone, isOverridden } = useBrowserLocale();
 * // keyboardLayout: "fr" for French browser (or from ?keyboard=fr)
 * // timezone: "Europe/Paris"
 * // isOverridden: true if using query param
 * ```
 */
export function useBrowserLocale(): BrowserLocaleInfo {
  const language = navigator.language || 'en-US';
  const override = getKeyboardOverride();
  const keyboardLayout = override || deriveKeyboardLayout(language);
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';

  return { language, keyboardLayout, timezone, isOverridden: !!override };
}

/**
 * Non-hook version for use outside React components.
 * Same logic as useBrowserLocale but can be called anywhere.
 */
export function getBrowserLocale(): BrowserLocaleInfo {
  const language = navigator.language || 'en-US';
  const override = getKeyboardOverride();
  const keyboardLayout = override || deriveKeyboardLayout(language);
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';

  return { language, keyboardLayout, timezone, isOverridden: !!override };
}
